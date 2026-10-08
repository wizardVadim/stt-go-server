package transcription

import (
	"strings"
	"time"
)

type Segment struct {
	start time.Duration
	end   time.Duration
	text  string
}

func NewSegment(start, end time.Duration, text string) (Segment, error) {
	s := Segment{start: start, end: end, text: text}
	if err := s.validate(); err != nil {
		return Segment{}, err
	}
	return s, nil
}
func (s Segment) validate() error {
	if strings.TrimSpace(s.text) == "" {
		return ErrInvalidSegmentText
	}
	if s.start < 0 || s.end < s.start {
		return ErrInvalidSegmentTime
	}
	return nil
}
func (s Segment) Start() time.Duration { return s.start }
func (s Segment) End() time.Duration   { return s.end }
func (s Segment) Text() string         { return s.text }

type Transcript struct {
	text     string
	segments []Segment
}

func NewTranscript(text string, segments []Segment, duration time.Duration) (Transcript, error) {
	if duration <= 0 || duration > MaxDuration {
		return Transcript{}, ErrInvalidDuration
	}
	var previousEnd time.Duration
	for _, s := range segments {
		if err := s.validate(); err != nil {
			return Transcript{}, err
		}
		if s.start < previousEnd || s.end > duration {
			return Transcript{}, ErrInvalidSegmentTime
		}
		previousEnd = s.end
	}
	if len(segments) > 0 && strings.TrimSpace(text) == "" {
		return Transcript{}, ErrInvalidSegmentText
	}
	return Transcript{text: text, segments: append([]Segment(nil), segments...)}, nil
}
func (t Transcript) Text() string        { return t.text }
func (t Transcript) Segments() []Segment { return append([]Segment(nil), t.segments...) }
