package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"stt-service/internal/core/domains/transcription"
)

func (s *JobService) RunCleanup(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		if err := s.Cleanup(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
			slog.Error("cleanup failed", "kind", "storage")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *JobService) Cleanup(ctx context.Context, now time.Time) error {
	s.mu.Lock()
	jobs, err := s.jobRepository.List(ctx)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	for _, j := range jobs {
		if j.Status().IsTerminal() && j.FinishedAt() != nil && now.Sub(*j.FinishedAt()) >= s.cfg.Retention {
			if e := s.removeLocked(ctx, j); e != nil {
				err = errors.Join(err, e)
			}
		}
	}
	// Only UUID-owned directories are eligible. A failed database lookup never
	// authorizes deletion. Submit uses the same mutex around rename + INSERT.
	entries, e := os.ReadDir(filepath.Join(s.cfg.DataDir, "jobs"))
	if e != nil {
		err = errors.Join(err, e)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, e := uuid.Parse(entry.Name()); e != nil {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			err = errors.Join(err, e)
			continue
		}
		if now.Sub(info.ModTime()) < s.cfg.Retention {
			continue
		}
		_, e = s.jobRepository.GetByID(ctx, entry.Name())
		if errors.Is(e, transcription.ErrJobNotFound) {
			err = errors.Join(err, os.RemoveAll(s.jobDir(entry.Name())))
		} else if e != nil {
			err = errors.Join(err, e)
		}
	}
	s.mu.Unlock()
	// Never race a multipart upload, even if it has been in progress for a day.
	select {
	case s.uploadGate <- struct{}{}:
		defer func() { <-s.uploadGate }()
	default:
		return err
	}
	entries, e = os.ReadDir(filepath.Join(s.cfg.DataDir, "tmp"))
	if e != nil {
		return errors.Join(err, e)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			err = errors.Join(err, e)
			continue
		}
		if now.Sub(info.ModTime()) >= s.cfg.Retention {
			err = errors.Join(err, os.Remove(filepath.Join(s.cfg.DataDir, "tmp", entry.Name())))
		}
	}
	return err
}
