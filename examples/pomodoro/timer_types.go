package main

import (
	"context"
	"io"
	"os"
	"time"
)

// main is the program entry point.
//
// T03 Phase A stub: it does not yet wire to runCLI. Phase B refactors it to
// osExit(runCLI(os.Args[1:], os.Stdout)) so the CLI and the timer engine are
// exercised through the real entry point.
func main() {}

// osExit is a seam for os.Exit so tests can capture the exit status without
// terminating the test process.
var osExit = os.Exit

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
	focusDuration time.Duration
	shortDuration time.Duration
	longDuration  time.Duration
	clock         Clock
	out           io.Writer
	// bellOut receives the R5 session-end bell (\a) once per transition. It is
	// separate from out so the in-place status-line stream on out stays intact;
	// it defaults to os.Stderr (the terminal's standard bell channel).
	bellOut        io.Writer
	completedFocus int
	currentSession int
}

// runTimerFn is the seam for the main execution logic.
var runTimerFn = func(ctx context.Context, focus, short, long time.Duration) error {
	return nil // stub
}
