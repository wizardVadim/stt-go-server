package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"stt-service/internal/core/domains/transcription"
	"stt-service/internal/process"
)

// Result is the stored/output representation, distinct from the domain model.
type Result struct {
	Language string          `json:"language"`
	Text     string          `json:"text"`
	Segments []ResultSegment `json:"segments"`
}
type ResultSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

func (s *JobService) Transcript(ctx context.Context, id string) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, err := s.jobRepository.GetByID(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if j.RemovedAt() != nil {
		return Result{}, transcription.ErrJobNotFound
	}
	if j.Status() != transcription.JobCompleted {
		return Result{}, ErrConflict
	}
	data, err := os.ReadFile(filepath.Join(s.jobDir(id), "transcript.json"))
	if err != nil {
		return Result{}, err
	}
	var result Result
	err = json.Unmarshal(data, &result)
	return result, err
}

// Recover runs before serving requests. Interrupted jobs can be explicitly retried.
func (s *JobService) Recover(ctx context.Context) error {
	jobs, err := s.jobRepository.List(ctx)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.Status() == transcription.JobPreparing || j.Status() == transcription.JobTranscribing {
			j, err = change(j, transcription.JobFailed, j.Progress(), "interrupted", "Обработка прервана перезапуском сервера. Повторите задание.")
			if err != nil {
				return err
			}
			if err = s.jobRepository.Update(ctx, j); err != nil {
				return err
			}
		}
	}
	return nil
}

// Run owns the only worker. The caller waits for it during shutdown.
func (s *JobService) Run(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.runNext(ctx); err != nil {
			slog.Error("worker operation failed", "kind", "storage_or_state")
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (s *JobService) runNext(parent context.Context) error {
	s.mu.Lock()
	jobs, err := s.jobRepository.List(parent)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	var job transcription.Job
	found := false
	for i := len(jobs) - 1; i >= 0; i-- {
		if jobs[i].Status() == transcription.JobQueued {
			job = jobs[i]
			found = true
			break
		}
	}
	if !found {
		s.mu.Unlock()
		return nil
	}
	job, err = change(job, transcription.JobPreparing, nil, "", "")
	if err == nil {
		err = s.jobRepository.Update(parent, job)
	}
	if err != nil {
		s.mu.Unlock()
		return err
	}
	ctx, cancel := context.WithTimeout(parent, s.cfg.ProcessTimeout)
	active := &activeJob{id: job.ID().String(), cancel: cancel, done: make(chan struct{})}
	s.active = active
	s.mu.Unlock()
	defer cancel()
	result, runErr := s.recognize(ctx, job)
	s.mu.Lock()
	defer s.mu.Unlock()
	defer close(active.done)
	defer func() { s.active = nil }()
	// Persist the final state even when the job or server context was cancelled.
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saveCancel()
	job, err = s.jobRepository.GetByID(saveCtx, job.ID().String())
	if err != nil {
		return err
	}
	status := transcription.JobCompleted
	code, message := "", ""
	p := 1.0
	progress := &p
	switch {
	case parent.Err() != nil:
		status = transcription.JobFailed
		code = "interrupted"
		message = "Обработка прервана остановкой сервера. Повторите задание."
		progress = job.Progress()
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		status = transcription.JobFailed
		code = "processing_timeout"
		message = "Превышено время обработки. Повторите задание."
		progress = job.Progress()
	case ctx.Err() != nil:
		status = transcription.JobCancelled
		progress = job.Progress()
	case runErr != nil:
		status = transcription.JobFailed
		code = "processing_failed"
		message = "Не удалось подготовить или распознать запись. Проверьте файл и повторите."
		progress = job.Progress()
		if errors.Is(runErr, ErrNoSpace) {
			code = "insufficient_space"
			message = "Недостаточно свободного места для обработки."
		}
	default:
		if err = s.saveResult(job.ID().String(), result); err != nil {
			status = transcription.JobFailed
			code = "result_save_failed"
			message = "Не удалось сохранить результат."
			progress = job.Progress()
		}
	}
	job, err = change(job, status, progress, code, message)
	if err != nil {
		return err
	}
	if err = s.jobRepository.Update(saveCtx, job); err != nil {
		return err
	}
	// Keep only the source and canonical result; partial engine outputs are disposable.
	for _, name := range []string{"audio.wav", "whisper.json"} {
		_ = os.Remove(filepath.Join(s.jobDir(job.ID().String()), name))
	}
	return nil
}
func (s *JobService) recognize(ctx context.Context, j transcription.Job) (Result, error) {
	dir := s.jobDir(j.ID().String())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Result{}, err
	}
	if err := checkSpace(dir, uint64(s.cfg.MinFreeBytes)+uint64(j.Duration().Seconds()*32000)+1<<20); err != nil {
		return Result{}, err
	}
	wav := filepath.Join(dir, "audio.wav")
	cmd := process.Command(ctx, s.cfg.FFmpeg, "-nostdin", "-v", "error", "-y", "-protocol_whitelist", "file,pipe", "-i", j.SourcePath(), "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav)
	if err := cmd.Run(); err != nil {
		return Result{}, err
	}
	s.mu.Lock()
	next, err := change(j, transcription.JobTranscribing, nil, "", "")
	if err == nil {
		err = s.jobRepository.Update(ctx, next)
	}
	s.mu.Unlock()
	if err != nil {
		return Result{}, err
	}
	prefix := filepath.Join(dir, "whisper")
	cmd = process.Command(ctx, s.cfg.Whisper, "-m", s.cfg.Model, "-f", wav, "-l", "en", "-t", strconv.Itoa(s.cfg.Threads), "-ng", "-oj", "-of", prefix)
	// Only timestamp boundaries are consumed; recognized speech is never logged.
	cmd.Stdout = &progressWriter{update: func(seconds float64) { s.recordProgress(ctx, j, seconds) }}
	if err = cmd.Run(); err != nil {
		return Result{}, err
	}
	data, err := os.ReadFile(prefix + ".json")
	if err != nil {
		return Result{}, err
	}
	return decodeWhisper(data, j.Duration())
}
func (s *JobService) recordProgress(ctx context.Context, j transcription.Job, seconds float64) {
	p := seconds / j.Duration().Seconds()
	if p < 0 || p > 1 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.jobRepository.GetByID(ctx, j.ID().String())
	if err != nil || current.Status() != transcription.JobTranscribing {
		return
	}
	if old := current.Progress(); old != nil && p <= *old {
		return
	}
	next, err := change(current, transcription.JobTranscribing, &p, "", "")
	if err == nil {
		_ = s.jobRepository.Update(ctx, next)
	}
}

var segmentLine = regexp.MustCompile(`^\s*\[\d{2}:\d{2}:\d{2}\.\d{3}\s+-->\s+(\d{2}):(\d{2}):(\d{2})\.(\d{3})\]`)

type progressWriter struct {
	buffer []byte
	update func(float64)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	for _, b := range p {
		if b == '\n' {
			if m := segmentLine.FindSubmatch(w.buffer); m != nil {
				v := make([]int, 4)
				for i := range v {
					v[i], _ = strconv.Atoi(string(m[i+1]))
				}
				w.update(float64(v[0]*3600+v[1]*60+v[2]) + float64(v[3])/1000)
			}
			w.buffer = w.buffer[:0]
		} else if len(w.buffer) < 65536 {
			w.buffer = append(w.buffer, b)
		}
	}
	return n, nil
}
func decodeWhisper(data []byte, duration time.Duration) (Result, error) {
	var raw struct {
		Transcription *[]struct {
			Offsets struct {
				From int64 `json:"from"`
				To   int64 `json:"to"`
			} `json:"offsets"`
			Text string `json:"text"`
		} `json:"transcription"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Result{}, err
	}
	if raw.Transcription == nil {
		return Result{}, fmt.Errorf("missing transcription")
	}
	result := Result{Language: "en", Segments: make([]ResultSegment, 0)}
	segments := make([]transcription.Segment, 0)
	var text strings.Builder
	for _, item := range *raw.Transcription {
		text.WriteString(item.Text)
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		if item.Offsets.From < 0 || item.Offsets.To < item.Offsets.From || item.Offsets.To > duration.Milliseconds()+20 {
			return Result{}, transcription.ErrInvalidSegmentTime
		}
		start, end := time.Duration(item.Offsets.From)*time.Millisecond, time.Duration(item.Offsets.To)*time.Millisecond
		// whisper timestamp ticks are 10 ms; tolerate only terminal quantization.
		if end > duration {
			end = duration
		}
		seg, err := transcription.NewSegment(start, end, item.Text)
		if err != nil {
			return Result{}, err
		}
		segments = append(segments, seg)
		result.Segments = append(result.Segments, ResultSegment{Start: start.Seconds(), End: end.Seconds(), Text: item.Text})
	}
	result.Text = text.String()
	if _, err := transcription.NewTranscript(result.Text, segments, duration); err != nil {
		return Result{}, err
	}
	return result, nil
}
func (s *JobService) saveResult(id string, result Result) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	dir := s.jobDir(id)
	path := filepath.Join(dir, "transcript.json.tmp")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(path, filepath.Join(dir, "transcript.json")); err != nil {
		return err
	}
	return syncDirectory(dir)
}
