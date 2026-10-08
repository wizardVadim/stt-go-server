package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"stt-service/internal/config"
	"stt-service/internal/features/transcription/repository"
	"stt-service/internal/features/transcription/service"
	"stt-service/internal/features/transcription/transport"
	"stt-service/internal/storage"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	syscall.Umask(0077)
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	for _, binary := range []string{cfg.FFprobe, cfg.FFmpeg, cfg.Whisper} {
		if _, err := exec.LookPath(binary); err != nil {
			return fmt.Errorf("required executable unavailable: %w", err)
		}
	}
	model, err := os.Open(cfg.Model)
	if err != nil {
		return fmt.Errorf("open model: %w", err)
	}
	info, err := model.Stat()
	model.Close()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("model must be a nonempty regular file")
	}
	if err = os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(cfg.DataDir, "server.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("another server owns the data directory")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := storage.Open(ctx, cfg.DBPath, cfg.MigrationsDir)
	if err != nil {
		return err
	}
	defer db.Close()
	jobs, err := service.NewJobService(repository.NewSQLiteRepository(db), cfg)
	if err != nil {
		return err
	}
	if err = jobs.Recover(ctx); err != nil {
		return err
	}
	workersCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); jobs.Run(workersCtx) }()
	go func() { defer wg.Done(); jobs.RunCleanup(workersCtx) }()
	server := &http.Server{Addr: cfg.Listen, Handler: transport.NewHandler(jobs, cfg), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if e := server.Shutdown(shutdownCtx); e != nil {
		_ = server.Close()
		if err == nil {
			err = e
		}
	}
	wg.Wait()
	return err
}
