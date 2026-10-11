package main

import (
	"context"
	"io"
	"time"
)

// main is provided for contract verification.
func main() {}

// Clock defines the interface for time-related operations to allow for testing.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// TimerState represents the current phase of the Pomodoro timer.
type TimerState int

const (
	Focus TimerState = iota
	ShortBreak
	LongBreak
)

// Timer manages the Pomodoro countdown and session transitions.
type Timer struct {
	focusDuration  time.Duration
	shortDuration  time.Duration
	longDuration   time.Duration
	clock          Clock
	out            io.Writer
	completedFocus int
	currentSession int
}

// runTimerFn is the seam for the main execution logic.
var runTimerFn = func(ctx context.Context, focus, short, long time.Duration) error {
	return nil // stub
}
