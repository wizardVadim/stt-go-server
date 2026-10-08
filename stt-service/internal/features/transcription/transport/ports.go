package transport

import (
	"context"
	"io"
	"stt-service/internal/core/domains/transcription"
)

type JobService interface {
	Create(ctx context.Context, filename string, src io.Reader) (transcription.Job, error)
}
