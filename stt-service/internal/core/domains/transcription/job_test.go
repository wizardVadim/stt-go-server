package transcription

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

// jobInput is a test fixture for exercising individual constructor arguments.
type jobInput struct {
	ID                       uuid.UUID
	Status                   JobStatus
	OriginalName, SourcePath string
	SizeBytes                int64
	Duration                 time.Duration
	Progress                 *float64
	ErrorCode, ErrorMessage  string
	CreatedAt, UpdatedAt     time.Time
	FinishedAt, RemovedAt    *time.Time
}

func constructJob(s jobInput) (Job, error) {
	return NewJob(s.ID, s.Status, s.OriginalName, s.SourcePath, s.SizeBytes,
		s.Duration, s.Progress, s.ErrorCode, s.ErrorMessage, s.CreatedAt,
		s.UpdatedAt, s.FinishedAt, s.RemovedAt)
}

func ptr[T any](v T) *T { return &v }
func validState() jobInput {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	return jobInput{ID: uuid.New(), Status: JobQueued, OriginalName: "lecture.mp4", SourcePath: "/data/source.mp4", SizeBytes: 100, Duration: time.Minute, Progress: ptr(0.0), CreatedAt: now, UpdatedAt: now}
}

func TestNewJob(t *testing.T) {
	s := validState()
	j, err := constructJob(s)
	if err != nil {
		t.Fatal(err)
	}
	if j.ID() != s.ID || j.Status() != JobQueued || j.OriginalName() != s.OriginalName || j.SourcePath() != s.SourcePath || j.SizeBytes() != s.SizeBytes || j.Duration() != s.Duration {
		t.Fatal("incorrect source metadata")
	}
	if j.Progress() == nil || *j.Progress() != 0 || j.ErrorCode() != "" || j.ErrorMessage() != "" || j.FinishedAt() != nil || j.CreatedAt() != s.CreatedAt || j.UpdatedAt() != s.CreatedAt {
		t.Fatal("incorrect initial state")
	}
}

func TestJobInvariants(t *testing.T) {
	tests := []struct {
		name   string
		change func(*jobInput)
		want   error
	}{
		{"nil id", func(s *jobInput) { s.ID = uuid.Nil }, ErrIdIsNil},
		{"blank filename", func(s *jobInput) { s.OriginalName = " \t" }, ErrEmptyOriginalName},
		{"blank path", func(s *jobInput) { s.SourcePath = " " }, ErrEmptySourcePath},
		{"empty file", func(s *jobInput) { s.SizeBytes = 0 }, ErrInvalidSize},
		{"negative size", func(s *jobInput) { s.SizeBytes = -1 }, ErrInvalidSize},
		{"unknown duration", func(s *jobInput) { s.Duration = 0 }, ErrInvalidDuration},
		{"negative duration", func(s *jobInput) { s.Duration = -1 }, ErrInvalidDuration},
		{"below limit", func(s *jobInput) { s.Duration = MaxDuration - time.Nanosecond }, nil},
		{"exact limit", func(s *jobInput) { s.Duration = MaxDuration }, nil},
		{"above limit", func(s *jobInput) { s.Duration = MaxDuration + time.Nanosecond }, ErrInvalidDuration},
		{"unknown status", func(s *jobInput) { s.Status = "running" }, ErrInvalidStatus},
		{"unknown progress", func(s *jobInput) { s.Progress = nil }, nil},
		{"negative progress", func(s *jobInput) { s.Progress = ptr(-0.1) }, ErrInvalidProgress},
		{"excess progress", func(s *jobInput) { s.Progress = ptr(1.1) }, ErrInvalidProgress},
		{"nan progress", func(s *jobInput) { s.Progress = ptr(math.NaN()) }, ErrInvalidProgress},
		{"infinite progress", func(s *jobInput) { s.Progress = ptr(math.Inf(1)) }, ErrInvalidProgress},
		{"negative infinite progress", func(s *jobInput) { s.Progress = ptr(math.Inf(-1)) }, ErrInvalidProgress},
		{"queued with progress", func(s *jobInput) { s.Progress = ptr(0.1) }, ErrInvalidProgress},
		{"preparing with progress", func(s *jobInput) { s.Status = JobPreparing; s.Progress = ptr(0.1) }, ErrInvalidProgress},
		{"no creation time", func(s *jobInput) { s.CreatedAt = time.Time{} }, ErrInvalidJobTime},
		{"no update time", func(s *jobInput) { s.UpdatedAt = time.Time{} }, ErrInvalidJobTime},
		{"update before creation", func(s *jobInput) { s.UpdatedAt = s.CreatedAt.Add(-time.Second) }, ErrInvalidJobTime},
		{"premature completion time", func(s *jobInput) { s.FinishedAt = ptr(s.CreatedAt) }, ErrInvalidJobTime},
		{"unexpected error", func(s *jobInput) { s.ErrorMessage = "failure" }, ErrInvalidJobError},
		{"unexpected error code", func(s *jobInput) { s.ErrorCode = "failure" }, ErrInvalidJobError},
		{"failed without error", func(s *jobInput) { s.Status = JobFailed; s.FinishedAt = ptr(s.UpdatedAt) }, ErrInvalidJobError},
		{"failed without code", func(s *jobInput) { s.Status = JobFailed; s.FinishedAt = ptr(s.UpdatedAt); s.ErrorMessage = "failure" }, ErrInvalidJobError},
		{"failed without message", func(s *jobInput) {
			s.Status = JobFailed
			s.FinishedAt = ptr(s.UpdatedAt)
			s.ErrorCode = "failure"
			s.ErrorMessage = " "
		}, ErrInvalidJobError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validState()
			tt.change(&s)
			_, err := constructJob(s)
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRestoreStatuses(t *testing.T) {
	for _, status := range []JobStatus{JobQueued, JobPreparing, JobTranscribing, JobCompleted, JobFailed, JobCancelled} {
		t.Run(string(status), func(t *testing.T) {
			s := validState()
			s.Status = status
			if status.IsTerminal() {
				s.FinishedAt = ptr(s.UpdatedAt)
			}
			if status == JobTranscribing {
				s.Progress = ptr(0.5)
			}
			if status == JobCompleted {
				s.Progress = ptr(1.0)
				s.FinishedAt = ptr(s.UpdatedAt)
			}
			if status == JobFailed {
				s.ErrorCode = "recognition_failed"
				s.ErrorMessage = "Recognition failed"
			}
			j, err := constructJob(s)
			if err != nil {
				t.Fatal(err)
			}
			if j.Status() != status || j.ErrorCode() != s.ErrorCode || j.ErrorMessage() != s.ErrorMessage {
				t.Fatal("state was not preserved")
			}
		})
	}
}

func TestCompletedJobInvariants(t *testing.T) {
	tests := []struct {
		name   string
		change func(*jobInput)
		want   error
	}{
		{"missing progress", func(s *jobInput) { s.Progress = nil }, ErrInvalidProgress},
		{"incomplete progress", func(s *jobInput) { s.Progress = ptr(0.9) }, ErrInvalidProgress},
		{"missing completion time", func(s *jobInput) { s.FinishedAt = nil }, ErrInvalidJobTime},
		{"zero completion time", func(s *jobInput) { s.FinishedAt = ptr(time.Time{}) }, ErrInvalidJobTime},
		{"completed before creation", func(s *jobInput) { s.FinishedAt = ptr(s.CreatedAt.Add(-time.Second)) }, ErrInvalidJobTime},
		{"completed after update", func(s *jobInput) { s.FinishedAt = ptr(s.UpdatedAt.Add(time.Second)) }, ErrInvalidJobTime},
		{"completed with error", func(s *jobInput) { s.ErrorCode = "failure"; s.ErrorMessage = "failure" }, ErrInvalidJobError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validState()
			s.Status = JobCompleted
			s.Progress = ptr(1.0)
			s.FinishedAt = ptr(s.UpdatedAt)
			tt.change(&s)
			if _, err := constructJob(s); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestJobOwnsOptionalValues(t *testing.T) {
	s := validState()
	s.Status = JobCompleted
	s.Progress = ptr(1.0)
	s.FinishedAt = ptr(s.UpdatedAt)
	j, err := constructJob(s)
	if err != nil {
		t.Fatal(err)
	}
	*s.Progress = -1
	*s.FinishedAt = time.Time{}
	*j.Progress() = -2
	*j.FinishedAt() = time.Time{}
	if *j.Progress() != 1 || !j.FinishedAt().Equal(s.UpdatedAt) {
		t.Fatal("external mutation changed job")
	}
}

func TestRemoval(t *testing.T) {
	for _, status := range []JobStatus{JobQueued, JobPreparing, JobTranscribing, JobCompleted, JobFailed, JobCancelled} {
		t.Run(string(status), func(t *testing.T) {
			s := validState()
			s.Status = status
			if status.IsTerminal() {
				s.FinishedAt = ptr(s.UpdatedAt)
			}
			if status == JobCompleted {
				s.Progress = ptr(1.0)
			}
			if status == JobFailed {
				s.ErrorCode = "failed"
				s.ErrorMessage = "Failure"
			}
			j, err := constructJob(s)
			if err != nil {
				t.Fatal(err)
			}
			now := s.UpdatedAt.Add(25 * time.Hour)
			err = j.MarkRemoved(now)
			if !status.IsTerminal() {
				if !errors.Is(err, ErrInvalidStatus) || j.RemovedAt() != nil {
					t.Fatal("active job was removed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if j.Status() != status || !j.RemovedAt().Equal(now) || !j.UpdatedAt().Equal(now) || !j.FinishedAt().Equal(*s.FinishedAt) {
				t.Fatal("removal lost history")
			}
			*j.RemovedAt() = time.Time{}
			if !j.RemovedAt().Equal(now) {
				t.Fatal("removal time leaked")
			}
			if err := j.MarkRemoved(now.Add(time.Hour)); err != nil || !j.RemovedAt().Equal(now) {
				t.Fatal("removal is not idempotent")
			}
		})
	}
}

func TestRemovalTimeInvariants(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*jobInput)
	}{
		{"active", func(s *jobInput) { s.Status = JobQueued; s.FinishedAt = nil }},
		{"zero removal", func(s *jobInput) { s.RemovedAt = ptr(time.Time{}) }},
		{"before finish", func(s *jobInput) { s.RemovedAt = ptr(s.FinishedAt.Add(-time.Second)) }},
		{"after update", func(s *jobInput) { s.RemovedAt = ptr(s.UpdatedAt.Add(time.Second)) }},
		{"missing finish", func(s *jobInput) { s.FinishedAt = nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := validState()
			s.Status = JobCancelled
			s.FinishedAt = ptr(s.CreatedAt)
			s.RemovedAt = ptr(s.UpdatedAt)
			tt.change(&s)
			if _, err := constructJob(s); !errors.Is(err, ErrInvalidJobTime) {
				t.Fatalf("got %v", err)
			}
		})
	}
	s := validState()
	s.Status = JobCancelled
	s.FinishedAt = ptr(s.CreatedAt)
	j, err := constructJob(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.MarkRemoved(s.UpdatedAt.Add(-time.Second)); !errors.Is(err, ErrInvalidJobTime) || j.RemovedAt() != nil {
		t.Fatal("invalid removal changed job")
	}
	s.RemovedAt = ptr(s.UpdatedAt)
	j, err = constructJob(s)
	if err != nil {
		t.Fatal(err)
	}
	*s.RemovedAt = time.Time{}
	if !j.RemovedAt().Equal(s.UpdatedAt) {
		t.Fatal("restore leaked removal pointer")
	}
}
