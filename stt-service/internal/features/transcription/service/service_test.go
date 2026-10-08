package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stt-service/internal/config"
	"stt-service/internal/core/domains/transcription"
	"stt-service/internal/features/transcription/repository"
	"stt-service/internal/storage"
)

func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
func fixture(t *testing.T) (*JobService, config.Config) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(root, "data"), MaxUploadBytes: 1024, MinFreeBytes: 1, Threads: 1, Retention: 24 * time.Hour, CleanupInterval: 24 * time.Hour, ProcessTimeout: time.Minute}
	cfg.FFprobe = script(t, root, "ffprobe", `echo '{"streams":[{"codec_type":"audio"}],"format":{"duration":"1.0"}}'`)
	cfg.FFmpeg = script(t, root, "ffmpeg", `for last; do :; done; printf wav > "$last"`)
	cfg.Whisper = script(t, root, "whisper", `while [ "$#" -gt 0 ]; do if [ "$1" = '-of' ]; then shift; prefix="$1"; fi; shift; done
printf '[00:00:00.000 --> 00:00:01.000] Hello world.\n'
printf '%s' '{"transcription":[{"offsets":{"from":0,"to":1000},"text":" Hello world."}]}' > "$prefix.json"
`)
	db, err := storage.Open(context.Background(), filepath.Join(root, "jobs.sqlite"), "../../../../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc, err := NewJobService(repository.NewSQLiteRepository(db), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return svc, cfg
}
func TestCreateDurationAndCleanup(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        error
	}{
		{"below", "599.999", nil}, {"exact", "600", nil}, {"above", "600.000001", transcription.ErrInvalidDuration},
		{"unknown", "N/A", transcription.ErrInvalidDuration}, {"zero", "0", transcription.ErrInvalidDuration}, {"negative", "-1", transcription.ErrInvalidDuration},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, cfg := fixture(t)
			script(t, filepath.Dir(cfg.FFprobe), "ffprobe", fmt.Sprintf("echo '%s'", `{"streams":[{"codec_type":"audio"}],"format":{"duration":"`+tc.value+`"}}`))
			job, err := s.Create(context.Background(), "lecture.mp4", strings.NewReader("media"))
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			tmp, _ := os.ReadDir(filepath.Join(cfg.DataDir, "tmp"))
			if len(tmp) != 0 {
				t.Fatal("staging file leaked")
			}
			jobs, e := s.List(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			if tc.want != nil {
				if len(jobs) != 0 {
					t.Fatal("rejected job persisted")
				}
				dirs, _ := os.ReadDir(filepath.Join(cfg.DataDir, "jobs"))
				if len(dirs) != 0 {
					t.Fatal("rejected source leaked")
				}
			} else {
				if len(jobs) != 1 || jobs[0].ID() != job.ID() || job.Status() != transcription.JobQueued {
					t.Fatal("job not persisted")
				}
			}
		})
	}
}
func TestUploadFailures(t *testing.T) {
	for _, mode := range []string{"empty", "large", "noaudio", "corrupt", "space"} {
		t.Run(mode, func(t *testing.T) {
			s, cfg := fixture(t)
			input := "media"
			var want error
			switch mode {
			case "empty":
				input = ""
				want = ErrInvalidMedia
			case "large":
				input = strings.Repeat("x", 1025)
				want = ErrTooLarge
			case "noaudio":
				script(t, filepath.Dir(cfg.FFprobe), "ffprobe", `echo '{"streams":[],"format":{"duration":"1"}}'`)
				want = ErrNoAudio
			case "corrupt":
				script(t, filepath.Dir(cfg.FFprobe), "ffprobe", "exit 1")
				want = ErrInvalidMedia
			case "space":
				s.cfg.MinFreeBytes = 1 << 62
				want = ErrNoSpace
			}
			_, err := s.Create(context.Background(), "lecture", strings.NewReader(input))
			if !errors.Is(err, want) {
				t.Fatalf("got %v want %v", err, want)
			}
			entries, _ := os.ReadDir(filepath.Join(cfg.DataDir, "tmp"))
			if len(entries) != 0 {
				t.Fatal("upload leaked")
			}
		})
	}
}
func TestWorkerResultRemovalHistory(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	job, err := s.Create(ctx, "lecture.wav", strings.NewReader("media"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.runNext(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := s.Transcript(ctx, job.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != " Hello world." || len(result.Segments) != 1 || result.Segments[0].End != 1 {
		t.Fatalf("wrong result: %+v", result)
	}
	done, err := s.jobRepository.GetByID(ctx, job.ID().String())
	if err != nil || done.Status() != transcription.JobCompleted || *done.Progress() != 1 {
		t.Fatal("job not completed", err)
	}
	if err = s.Cleanup(ctx, time.Now().Add(25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	jobs, err := s.List(ctx)
	if err != nil || len(jobs) != 0 {
		t.Fatal("removed job visible", err)
	}
	history, err := s.jobRepository.GetByID(ctx, job.ID().String())
	if err != nil || history.RemovedAt() == nil || history.Status() != transcription.JobCompleted {
		t.Fatal("history lost", err)
	}
	if _, err = os.Stat(job.SourcePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source not removed")
	}
	if _, err = s.Transcript(ctx, job.ID().String()); !errors.Is(err, transcription.ErrJobNotFound) {
		t.Fatal(err)
	}
	if _, err = s.Retry(ctx, job.ID().String()); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
func TestCancelRetryRecover(t *testing.T) {
	s, cfg := fixture(t)
	ctx := context.Background()
	script(t, filepath.Dir(cfg.Whisper), "whisper", "sleep 30")
	job, err := s.Create(ctx, "lecture", strings.NewReader("media"))
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- s.runNext(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		j, e := s.jobRepository.GetByID(ctx, job.ID().String())
		if e != nil {
			t.Fatal(e)
		}
		if j.Status() == transcription.JobTranscribing {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancelCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cancelled, err := s.Cancel(cancelCtx, job.ID().String())
	if err != nil || cancelled.Status() != transcription.JobCancelled {
		t.Fatal("cancel failed", err)
	}
	if err = <-runDone; err != nil {
		t.Fatal(err)
	}
	retry, err := s.Retry(ctx, job.ID().String())
	if err != nil || retry.ID() != job.ID() || retry.FinishedAt() != nil || *retry.Progress() != 0 {
		t.Fatal("retry failed", err)
	}
	interrupted, err := change(retry, transcription.JobTranscribing, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.jobRepository.Update(ctx, interrupted); err != nil {
		t.Fatal(err)
	}
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.jobRepository.GetByID(ctx, job.ID().String())
	if err != nil || recovered.Status() != transcription.JobFailed || recovered.ErrorCode() != "interrupted" {
		t.Fatal("recovery failed", err)
	}
}

type failingRepository struct{ JobRepository }

func (f failingRepository) Create(context.Context, transcription.Job) error {
	return errors.New("insert failed")
}
func TestPersistenceFailureRemovesFile(t *testing.T) {
	s, cfg := fixture(t)
	s.jobRepository = failingRepository{s.jobRepository}
	if _, err := s.Create(context.Background(), "lecture", strings.NewReader("media")); err == nil {
		t.Fatal("expected failure")
	}
	for _, name := range []string{"tmp", "jobs"} {
		entries, _ := os.ReadDir(filepath.Join(cfg.DataDir, name))
		if len(entries) != 0 {
			t.Fatal("leaked files", name)
		}
	}
}
func TestCleanupProtectsActiveAndOrphans(t *testing.T) {
	s, cfg := fixture(t)
	ctx := context.Background()
	job, err := s.Create(ctx, "lecture", strings.NewReader("media"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Cleanup(ctx, time.Now().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(job.SourcePath()); err != nil {
		t.Fatal("queued source removed", err)
	}
	u, err := s.Stage(ctx, "another", strings.NewReader("media"))
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	if err = s.Cleanup(ctx, time.Now().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(u.path); err != nil {
		t.Fatal("active upload removed", err)
	}
	if err = u.Close(); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(cfg.DataDir, "tmp", "upload-orphan")
	if err = os.WriteFile(orphan, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.Cleanup(ctx, time.Now().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("orphan retained")
	}
}

func TestActualFFprobeAndFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	s, cfg := fixture(t)
	s.cfg.FFmpeg = ffmpeg
	s.cfg.FFprobe = probe
	input := filepath.Join(filepath.Dir(cfg.FFprobe), "input.wav")
	cmd := exec.Command(ffmpeg, "-nostdin", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-ac", "2", "-ar", "44100", input)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate audio: %v %s", err, out)
	}
	f, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s.cfg.MaxUploadBytes = 1 << 20
	job, err := s.Create(context.Background(), "test.wav", f)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.runNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, err := s.Transcript(context.Background(), job.ID().String())
	if err != nil || result.Text != " Hello world." {
		t.Fatal("pipeline failed", err)
	}
	_, err = s.Create(context.Background(), "corrupt.wav", strings.NewReader("not audio"))
	if !errors.Is(err, ErrInvalidMedia) {
		t.Fatal("corrupt media accepted", err)
	}
}
