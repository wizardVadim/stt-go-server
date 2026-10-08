package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"stt-service/internal/config"
	"stt-service/internal/core/domains/transcription"
	"stt-service/internal/process"

	"github.com/google/uuid"
)

var (
	ErrTooLarge     = errors.New("file exceeds upload limit")
	ErrInvalidMedia = errors.New("invalid media")
	ErrNoAudio      = errors.New("no audio track")
	ErrNoSpace      = errors.New("insufficient disk space")
	ErrConflict     = errors.New("action unavailable")
)

type activeJob struct {
	id     string
	cancel context.CancelFunc
	done   chan struct{}
}
type JobService struct {
	jobRepository JobRepository
	cfg           config.Config
	mu            sync.Mutex // serializes state transitions, completion, deletion and retry
	uploadGate    chan struct{}
	active        *activeJob
}

func NewJobService(repo JobRepository, cfg config.Config) (*JobService, error) {
	for _, dir := range []string{filepath.Join(cfg.DataDir, "tmp"), filepath.Join(cfg.DataDir, "jobs")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	return &JobService{jobRepository: repo, cfg: cfg, uploadGate: make(chan struct{}, 1)}, nil
}

// Upload owns a staging file and the upload slot until Close. The HTTP adapter
// can validate trailing multipart fields before accepting a durable job.
type Upload struct {
	path, name string
	size       int64
	service    *JobService
	once       sync.Once
}

func (u *Upload) Close() error {
	var err error
	u.once.Do(func() {
		err = os.Remove(u.path)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		<-u.service.uploadGate
	})
	return err
}
func (s *JobService) Stage(ctx context.Context, name string, src io.Reader) (*Upload, error) {
	if strings.TrimSpace(name) == "" || src == nil {
		return nil, ErrInvalidMedia
	}
	select {
	case s.uploadGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	path, size, err := saveUpload(ctx, filepath.Join(s.cfg.DataDir, "tmp"), src, s.cfg.MaxUploadBytes, s.cfg.MinFreeBytes)
	if err != nil {
		<-s.uploadGate
		return nil, err
	}
	return &Upload{path: path, name: name, size: size, service: s}, nil
}
func (s *JobService) Create(ctx context.Context, name string, src io.Reader) (transcription.Job, error) {
	u, err := s.Stage(ctx, name, src)
	if err != nil {
		return transcription.Job{}, err
	}
	defer u.Close()
	return s.Submit(ctx, u)
}
func (s *JobService) Submit(ctx context.Context, u *Upload) (job transcription.Job, err error) {
	if u == nil || u.service != s {
		return job, ErrInvalidMedia
	}
	seconds, err := probeMedia(ctx, s.cfg.FFprobe, u.path)
	if err != nil {
		return job, err
	}
	if seconds > transcription.MaxDuration.Seconds() {
		return job, transcription.ErrInvalidDuration
	}
	if err = ctx.Err(); err != nil {
		return job, err
	}
	id := uuid.New()
	dir := s.jobDir(id.String())
	source := filepath.Join(dir, "source")
	now := time.Now().UTC()
	progress := 0.0
	job, err = transcription.NewJob(id, transcription.JobQueued, u.name, source, u.size, time.Duration(math.Round(seconds*float64(time.Second))), &progress, "", "", now, now, nil, nil)
	if err != nil {
		return transcription.Job{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = os.Mkdir(dir, 0700); err != nil {
		return transcription.Job{}, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(dir))
		}
	}()
	if err = os.Rename(u.path, source); err != nil {
		return transcription.Job{}, err
	}
	if err = syncDirectory(dir); err != nil {
		return transcription.Job{}, err
	}
	if err = syncDirectory(filepath.Dir(dir)); err != nil {
		return transcription.Job{}, err
	}

	if err = s.jobRepository.Create(ctx, job); err != nil {
		return transcription.Job{}, err
	}
	return job, nil
}
func (s *JobService) jobDir(id string) string { return filepath.Join(s.cfg.DataDir, "jobs", id) }

func checkSpace(path string, reserve uint64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return err
	}
	if stat.Bavail*uint64(stat.Bsize) < reserve {
		return ErrNoSpace
	}
	return nil
}

type uploadWriter struct {
	ctx     context.Context
	file    *os.File
	dir     string
	reserve int64
}

func (w uploadWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if err := checkSpace(w.dir, uint64(w.reserve)+uint64(len(p))); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}
func saveUpload(ctx context.Context, dir string, src io.Reader, maxBytes, reserve int64) (path string, size int64, err error) {
	if maxBytes <= 0 {
		return "", 0, ErrTooLarge
	}
	f, err := os.CreateTemp(dir, "upload-*")
	if err != nil {
		return "", 0, err
	}
	path = f.Name()
	defer func() {
		if err != nil {
			f.Close()
			if e := os.Remove(path); e != nil && !errors.Is(e, os.ErrNotExist) {
				err = errors.Join(err, e)
			}
		}
	}()
	size, err = io.Copy(uploadWriter{ctx, f, dir, reserve}, io.LimitReader(src, maxBytes))
	if err != nil {
		return path, size, err
	}
	var extra [1]byte
	n, e := io.ReadFull(src, extra[:])
	if n > 0 {
		return path, size, ErrTooLarge
	}
	if e != nil && !errors.Is(e, io.EOF) {
		return path, size, e
	}
	if size == 0 {
		return path, size, ErrInvalidMedia
	}
	if err = f.Sync(); err != nil {
		return path, size, err
	}
	err = f.Close()
	return path, size, err
}
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func probeMedia(ctx context.Context, binary, path string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := process.Command(ctx, binary, "-v", "error", "-protocol_whitelist", "file,pipe", "-select_streams", "a:0", "-show_entries", "stream=codec_type:format=duration", "-of", "json", path)
	cmd.Stdout = nil
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, ErrInvalidMedia
	}
	var result struct {
		Streams []struct {
			Type string `json:"codec_type"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if json.Unmarshal(out, &result) != nil {
		return 0, ErrInvalidMedia
	}
	if len(result.Streams) == 0 || result.Streams[0].Type != "audio" {
		return 0, ErrNoAudio
	}
	seconds, e := strconv.ParseFloat(result.Format.Duration, 64)
	if e != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return 0, transcription.ErrInvalidDuration
	}
	return seconds, nil
}

func (s *JobService) List(ctx context.Context) ([]transcription.Job, error) {
	return s.jobRepository.List(ctx)
}
func change(j transcription.Job, status transcription.JobStatus, p *float64, code, message string) (transcription.Job, error) {
	now := time.Now().UTC()
	var finished *time.Time
	if status.IsTerminal() {
		finished = &now
	}
	return transcription.NewJob(j.ID(), status, j.OriginalName(), j.SourcePath(), j.SizeBytes(), j.Duration(), p, code, message, j.CreatedAt(), now, finished, j.RemovedAt())
}
func (s *JobService) Cancel(ctx context.Context, id string) (transcription.Job, error) {
	s.mu.Lock()
	j, err := s.jobRepository.GetByID(ctx, id)
	if err != nil {
		s.mu.Unlock()
		return j, err
	}
	if j.RemovedAt() != nil || j.Status().IsTerminal() {
		s.mu.Unlock()
		return transcription.Job{}, ErrConflict
	}
	if s.active != nil && s.active.id == id {
		active := s.active
		active.cancel()
		s.mu.Unlock()
		// Do not acknowledge cancellation until the child process has exited.
		select {
		case <-active.done:
		case <-ctx.Done():
			return transcription.Job{}, ctx.Err()
		}
		return s.jobRepository.GetByID(ctx, id)
	}
	j, err = change(j, transcription.JobCancelled, j.Progress(), "", "")
	if err == nil {
		err = s.jobRepository.Update(ctx, j)
	}
	s.mu.Unlock()
	return j, err
}
func (s *JobService) Retry(ctx context.Context, id string) (transcription.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, err := s.jobRepository.GetByID(ctx, id)
	if err != nil {
		return j, err
	}
	if j.RemovedAt() != nil || (j.Status() != transcription.JobFailed && j.Status() != transcription.JobCancelled) {
		return transcription.Job{}, ErrConflict
	}
	if _, err = os.Stat(j.SourcePath()); err != nil {
		return transcription.Job{}, ErrConflict
	}
	if err = s.clearResults(id); err != nil {
		return transcription.Job{}, err
	}
	p := 0.0
	j, err = change(j, transcription.JobQueued, &p, "", "")
	if err == nil {
		err = s.jobRepository.Update(ctx, j)
	}
	return j, err
}
func (s *JobService) Remove(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, err := s.jobRepository.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return s.removeLocked(ctx, j)
}
func (s *JobService) removeLocked(ctx context.Context, j transcription.Job) error {
	if !j.Status().IsTerminal() {
		return ErrConflict
	}
	if j.RemovedAt() != nil {
		return nil
	}
	if err := os.RemoveAll(s.jobDir(j.ID().String())); err != nil {
		return err
	}
	// Historical source paths may predate the per-job directory layout.
	if err := os.Remove(j.SourcePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := j.MarkRemoved(time.Now().UTC()); err != nil {
		return err
	}
	return s.jobRepository.Update(ctx, j)
}
func (s *JobService) clearResults(id string) error {
	for _, name := range []string{"audio.wav", "whisper.json", "transcript.json", "transcript.json.tmp"} {
		if err := os.Remove(filepath.Join(s.jobDir(id), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
