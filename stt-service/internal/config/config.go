package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Listen, DataDir, DBPath, MigrationsDir, WebDir string
	FFprobe, FFmpeg, Whisper, Model                string
	MaxUploadBytes, MinFreeBytes                   int64
	Threads                                        int
	Retention, CleanupInterval, ProcessTimeout     time.Duration
}

func Load() (Config, error) {
	c := Config{
		Listen: env("STT_LISTEN", "127.0.0.1:8080"), DataDir: env("STT_DATA_DIR", "./data"),
		MigrationsDir: env("STT_MIGRATIONS_DIR", "../migrations"), WebDir: env("STT_WEB_DIR", "../web/dist"),
		FFprobe: env("STT_FFPROBE", "ffprobe"), FFmpeg: env("STT_FFMPEG", "ffmpeg"),
		Whisper: env("STT_WHISPER", "whisper-cli"), Model: os.Getenv("STT_MODEL"),
	}
	var err error
	c.DataDir, err = filepath.Abs(c.DataDir)
	if err != nil {
		return c, err
	}
	c.DBPath = env("STT_DB_PATH", filepath.Join(c.DataDir, "jobs.sqlite"))
	c.MaxUploadBytes, err = number("STT_MAX_UPLOAD_BYTES", 500<<20)
	if err != nil {
		return c, err
	}
	if c.MaxUploadBytes > 1<<40 {
		return c, fmt.Errorf("STT_MAX_UPLOAD_BYTES exceeds 1 TiB")
	}
	c.MinFreeBytes, err = number("STT_MIN_FREE_BYTES", 1<<30)
	if err != nil {
		return c, err
	}
	n, err := number("STT_THREADS", 4)
	if err != nil || n > 256 {
		return c, fmt.Errorf("STT_THREADS must be between 1 and 256")
	}
	c.Threads = int(n)
	c.Retention, err = duration("STT_RETENTION", 24*time.Hour)
	if err != nil {
		return c, err
	}
	c.CleanupInterval, err = duration("STT_CLEANUP_INTERVAL", 24*time.Hour)
	if err != nil {
		return c, err
	}
	c.ProcessTimeout, err = duration("STT_PROCESS_TIMEOUT", 24*time.Hour)
	if err != nil {
		return c, err
	}
	if c.Model == "" {
		return c, fmt.Errorf("STT_MODEL is required")
	}
	return c, nil
}
func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func number(k string, fallback int64) (int64, error) {
	v, err := strconv.ParseInt(env(k, strconv.FormatInt(fallback, 10)), 10, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", k)
	}
	return v, nil
}
func duration(k string, fallback time.Duration) (time.Duration, error) {
	v, err := time.ParseDuration(env(k, fallback.String()))
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", k)
	}
	return v, nil
}
