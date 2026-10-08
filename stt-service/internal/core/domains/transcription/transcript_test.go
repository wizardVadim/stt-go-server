package transcription

import (
	"errors"
	"testing"
	"time"
)

func TestSegmentValidation(t *testing.T) {
	for _, tt := range []struct {
		name       string
		start, end time.Duration
		text       string
		want       error
	}{
		{"valid", 0, time.Second, " Hello ", nil},
		{"zero length", time.Second, time.Second, "Hello", nil},
		{"negative start", -1, time.Second, "Hello", ErrInvalidSegmentTime},
		{"reversed", time.Second, 0, "Hello", ErrInvalidSegmentTime},
		{"blank", 0, time.Second, " \n", ErrInvalidSegmentText},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewSegment(tt.start, tt.end, tt.text)
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
			if err == nil && (s.Start() != tt.start || s.End() != tt.end || s.Text() != tt.text) {
				t.Fatal("segment changed")
			}
		})
	}
}

func TestTranscriptValidation(t *testing.T) {
	a, _ := NewSegment(0, time.Second, "Hello")
	b, _ := NewSegment(time.Second, 2*time.Second, "world")
	overlap, _ := NewSegment(time.Second/2, 2*time.Second, "world")
	for _, tt := range []struct {
		name, text string
		segments   []Segment
		duration   time.Duration
		want       error
	}{
		{"silence", "", nil, time.Minute, nil},
		{"no timestamps", "Hello", nil, time.Minute, nil},
		{"adjacent and exact boundary", "Hello world", []Segment{a, b}, 2 * time.Second, nil},
		{"overlap", "Hello world", []Segment{a, overlap}, time.Minute, ErrInvalidSegmentTime},
		{"out of order", "world Hello", []Segment{b, a}, time.Minute, ErrInvalidSegmentTime},
		{"outside media", "world", []Segment{b}, time.Second, ErrInvalidSegmentTime},
		{"zero segment", "Hello", []Segment{{}}, time.Minute, ErrInvalidSegmentText},
		{"missing full text", " ", []Segment{a}, time.Minute, ErrInvalidSegmentText},
		{"invalid duration", "", nil, 0, ErrInvalidDuration},
		{"too long", "", nil, MaxDuration + 1, ErrInvalidDuration},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTranscript(tt.text, tt.segments, tt.duration)
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestTranscriptOwnsSegments(t *testing.T) {
	a, _ := NewSegment(0, time.Second, "Hello")
	segments := []Segment{a}
	transcript, err := NewTranscript(" Hello\n", segments, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	segments[0] = Segment{}
	transcript.Segments()[0] = Segment{}
	if transcript.Text() != " Hello\n" || transcript.Segments()[0].Text() != "Hello" {
		t.Fatal("external mutation changed transcript")
	}
}
