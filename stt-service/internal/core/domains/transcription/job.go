package transcription

import (
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

const MaxDuration = 10 * time.Minute

type JobStatus string

const (
	JobQueued       JobStatus = "queued"
	JobPreparing    JobStatus = "preparing"
	JobTranscribing JobStatus = "transcribing"
	JobCompleted    JobStatus = "completed"
	JobFailed       JobStatus = "failed"
	JobCancelled    JobStatus = "cancelled"
)

type Job struct {
	id           uuid.UUID
	status       JobStatus
	originalName string
	sourcePath   string
	sizeBytes    int64
	duration     time.Duration
	progress     *float64
	errorCode    string
	errorMessage string
	createdAt    time.Time
	updatedAt    time.Time
	finishedAt   *time.Time
	removedAt    *time.Time
}

// NewJob constructs a new or persisted job from all its fields.
func NewJob(
	id uuid.UUID,
	status JobStatus,
	originalName, sourcePath string,
	sizeBytes int64,
	duration time.Duration,
	progress *float64,
	errorCode, errorMessage string,
	createdAt, updatedAt time.Time,
	finishedAt, removedAt *time.Time,
) (Job, error) {
	job := Job{
		id: id, status: status, originalName: originalName, sourcePath: sourcePath,
		sizeBytes: sizeBytes, duration: duration, progress: copyPointer(progress),
		errorCode: errorCode, errorMessage: errorMessage,
		createdAt: createdAt, updatedAt: updatedAt,
		finishedAt: copyPointer(finishedAt), removedAt: copyPointer(removedAt),
	}
	if err := validateJob(job); err != nil {
		return Job{}, err
	}
	return job, nil
}

func validateJob(s Job) error {
	if s.id == uuid.Nil {
		return ErrIdIsNil
	}
	if strings.TrimSpace(s.originalName) == "" {
		return ErrEmptyOriginalName
	}
	if strings.TrimSpace(s.sourcePath) == "" {
		return ErrEmptySourcePath
	}
	if s.sizeBytes <= 0 {
		return ErrInvalidSize
	}
	if s.duration <= 0 || s.duration > MaxDuration {
		return ErrInvalidDuration
	}
	switch s.status {
	case JobQueued, JobPreparing, JobTranscribing, JobCompleted, JobFailed, JobCancelled:
	default:
		return ErrInvalidStatus
	}
	if s.progress != nil {
		v := *s.progress
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return ErrInvalidProgress
		}
		if (s.status == JobQueued || s.status == JobPreparing) && v != 0 {
			return ErrInvalidProgress
		}
	}
	if s.createdAt.IsZero() || s.updatedAt.IsZero() || s.updatedAt.Before(s.createdAt) {
		return ErrInvalidJobTime
	}
	if s.status == JobCompleted {
		if s.progress == nil || *s.progress != 1 {
			return ErrInvalidProgress
		}
	}
	if s.status.IsTerminal() {
		if s.finishedAt == nil || s.finishedAt.IsZero() || s.finishedAt.Before(s.createdAt) || s.finishedAt.After(s.updatedAt) {
			return ErrInvalidJobTime
		}
	} else if s.finishedAt != nil {
		return ErrInvalidJobTime
	}
	if s.removedAt != nil {
		if !s.status.IsTerminal() || s.removedAt.IsZero() || s.removedAt.Before(*s.finishedAt) || s.removedAt.After(s.updatedAt) {
			return ErrInvalidJobTime
		}
	}
	if s.status == JobFailed {
		if strings.TrimSpace(s.errorCode) == "" || strings.TrimSpace(s.errorMessage) == "" {
			return ErrInvalidJobError
		}
	} else if s.errorCode != "" || s.errorMessage != "" {
		return ErrInvalidJobError
	}
	return nil
}

func copyPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func (j Job) ID() uuid.UUID           { return j.id }
func (j Job) Status() JobStatus       { return j.status }
func (j Job) OriginalName() string    { return j.originalName }
func (j Job) SourcePath() string      { return j.sourcePath }
func (j Job) SizeBytes() int64        { return j.sizeBytes }
func (j Job) Duration() time.Duration { return j.duration }
func (j Job) Progress() *float64      { return copyPointer(j.progress) }
func (j Job) ErrorCode() string       { return j.errorCode }
func (j Job) ErrorMessage() string    { return j.errorMessage }
func (j Job) CreatedAt() time.Time    { return j.createdAt }
func (j Job) UpdatedAt() time.Time    { return j.updatedAt }
func (j Job) FinishedAt() *time.Time  { return copyPointer(j.finishedAt) }

// IsTerminal reports whether processing has ended, regardless of its outcome.
func (s JobStatus) IsTerminal() bool {
	return s == JobCompleted || s == JobFailed || s == JobCancelled
}

func (j Job) RemovedAt() *time.Time { return copyPointer(j.removedAt) }

// MarkRemoved is called only after all associated files have been removed.
// It preserves the processing outcome and is idempotent.
func (j *Job) MarkRemoved(now time.Time) error {
	if !j.status.IsTerminal() {
		return ErrInvalidStatus
	}
	if j.removedAt != nil {
		return nil
	}
	if now.Before(j.updatedAt) {
		return ErrInvalidJobTime
	}
	updated := *j
	updated.removedAt = &now
	updated.updatedAt = now
	if err := validateJob(updated); err != nil {
		return err
	}
	*j = updated
	return nil
}
