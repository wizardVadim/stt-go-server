package service

import (
	"context"
	"stt-service/internal/core/domains/transcription"
)

type JobRepository interface {
	Create(ctx context.Context, job transcription.Job) error
	GetByID(ctx context.Context, id string) (transcription.Job, error)
	// List returns only jobs whose removed_at is NULL.
	List(ctx context.Context) ([]transcription.Job, error)
	Update(ctx context.Context, job transcription.Job) error
}
