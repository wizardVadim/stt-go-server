package transcription

import "errors"

var (
	ErrIdIsNil            = errors.New("id is nil")
	ErrEmptyOriginalName  = errors.New("original name is empty")
	ErrEmptySourcePath    = errors.New("source path is empty")
	ErrInvalidSize        = errors.New("source size must be positive")
	ErrInvalidDuration    = errors.New("duration must be positive and at most 600 seconds")
	ErrInvalidStatus      = errors.New("invalid job status")
	ErrInvalidProgress    = errors.New("invalid recognition progress for job status")
	ErrInvalidJobTime     = errors.New("inconsistent job timestamps")
	ErrInvalidJobError    = errors.New("only failed jobs must have an error code and message")
	ErrInvalidSegmentText = errors.New("segment text is empty")
	ErrInvalidSegmentTime = errors.New("invalid segment timestamps")
	ErrJobNotFound        = errors.New("job is not found")
)
