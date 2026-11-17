package timelib_test

import (
	"testing"
	"time"

	"golangApp/lib/timelib"
)

type mockTargetTime struct {
	calls []time.Time
}

func (m *mockTargetTime) Window(current time.Time) {
	m.calls = append(m.calls, current)
}

func (m *mockTargetTime) Count() int { return len(m.calls) }

func TestFromStartToEnd(t *testing.T) {
	target := &mockTargetTime{}
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 1, 1, 0, 0, 10, 0, time.UTC)
	window := time.Second

	timelib.FromStartToEnd(target, start, end, window)

	// Windows: 0,1,2,...,9 (10 windows, since start < end for 0..9, then start=10 which is not before end)
	if got := target.Count(); got != 10 {
		t.Fatalf("expected 10 windows, got %d", got)
	}
	if target.calls[0] != start {
		t.Fatalf("first window = %v, want %v", target.calls[0], start)
	}
	if target.calls[9] != time.Date(2020, 1, 1, 0, 0, 9, 0, time.UTC) {
		t.Fatalf("last window = %v, want 09:00", target.calls[9])
	}
}

func TestFromStartToEndStartEqualsEnd(t *testing.T) {
	target := &mockTargetTime{}
	tt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	timelib.FromStartToEnd(target, tt, tt, time.Second)
	if got := target.Count(); got != 0 {
		t.Fatalf("expected 0 windows when start==end, got %d", got)
	}
}

func TestFromStartToEndStartAfterEnd(t *testing.T) {
	target := &mockTargetTime{}
	start := time.Date(2020, 1, 1, 0, 0, 10, 0, time.UTC)
	end := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	timelib.FromStartToEnd(target, start, end, time.Second)
	if got := target.Count(); got != 0 {
		t.Fatalf("expected 0 windows when start>end, got %d", got)
	}
}

func TestFromEndToStart(t *testing.T) {
	target := &mockTargetTime{}
	start := time.Date(2020, 1, 1, 0, 0, 10, 0, time.UTC)
	end := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	window := time.Second

	timelib.FromEndToStart(target, start, end, window)

	// Windows: end, end+1s, ..., end+9s (10 windows)
	if got := target.Count(); got != 10 {
		t.Fatalf("expected 10 windows, got %d", got)
	}
	if target.calls[0] != end {
		t.Fatalf("first window = %v, want %v (end)", target.calls[0], end)
	}
	if target.calls[9] != time.Date(2020, 1, 1, 0, 0, 9, 0, time.UTC) {
		t.Fatalf("last window = %v, want 09:00", target.calls[9])
	}
}

func TestFromEndToStartStartEqualsEnd(t *testing.T) {
	target := &mockTargetTime{}
	tt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	timelib.FromEndToStart(target, tt, tt, time.Second)
	if got := target.Count(); got != 0 {
		t.Fatalf("expected 0 windows when start==end, got %d", got)
	}
}

func TestFromEndToStartStartBeforeEnd(t *testing.T) {
	target := &mockTargetTime{}
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 1, 1, 0, 0, 10, 0, time.UTC)
	timelib.FromEndToStart(target, start, end, time.Second)
	if got := target.Count(); got != 0 {
		t.Fatalf("expected 0 windows when start<end, got %d", got)
	}
}
