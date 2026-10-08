package repository

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"stt-service/internal/core/domains/transcription"
	"stt-service/internal/core/utils"

	"github.com/google/uuid"
)

type JobDAO struct {
	ID               string
	OriginalFilename string
	SourcePath       string
	SizeBytes        int64
	DurationSeconds  float64
	Status           string
	Progress         sql.NullFloat64
	ErrorCode        sql.NullString
	ErrorMessage     sql.NullString
	CreatedAt        string
	UpdatedAt        string
	FinishedAt       sql.NullString
	RemovedAt        sql.NullString
}

func (model JobDAO) ParseIntoDomain() (transcription.Job, error) {
	id, err := uuid.Parse(model.ID)
	if err != nil {
		return transcription.Job{}, fmt.Errorf("parse job ID: %w", err)
	}
	createdAt, err := time.Parse(utils.StorageTimeLayout, model.CreatedAt)
	if err != nil {
		return transcription.Job{}, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := time.Parse(utils.StorageTimeLayout, model.UpdatedAt)
	if err != nil {
		return transcription.Job{}, fmt.Errorf("parse updated_at: %w", err)
	}
	finishedAt, err := parseOptionalJobTime(model.FinishedAt)
	if err != nil {
		return transcription.Job{}, fmt.Errorf("parse finished_at: %w", err)
	}
	removedAt, err := parseOptionalJobTime(model.RemovedAt)
	if err != nil {
		return transcription.Job{}, fmt.Errorf("parse removed_at: %w", err)
	}
	// Validate seconds before conversion to avoid overflow or truncating excess duration.
	if math.IsNaN(model.DurationSeconds) || math.IsInf(model.DurationSeconds, 0) || model.DurationSeconds <= 0 || model.DurationSeconds > transcription.MaxDuration.Seconds() {
		return transcription.Job{}, transcription.ErrInvalidDuration
	}
	duration := time.Duration(math.Round(model.DurationSeconds * float64(time.Second)))
	var progress *float64
	if model.Progress.Valid {
		value := model.Progress.Float64
		progress = &value
	}
	var errorCode, errorMessage string
	if model.ErrorCode.Valid {
		errorCode = model.ErrorCode.String
	}
	if model.ErrorMessage.Valid {
		errorMessage = model.ErrorMessage.String
	}
	return transcription.NewJob(
		id, transcription.JobStatus(model.Status), model.OriginalFilename, model.SourcePath,
		model.SizeBytes, duration, progress, errorCode, errorMessage,
		createdAt, updatedAt, finishedAt, removedAt,
	)
}

func parseOptionalJobTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := time.Parse(utils.StorageTimeLayout, value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
