package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"time"
)

// main is the program entry point. It delegates to runCLI with the process
// arguments and standard output, passing the resulting exit code to osExit.
func main() {
	osExit(runCLI(os.Args[1:], os.Stdout))
}

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

// realClock is the production Clock: it observes wall-clock time and its
// After channel fires when the duration elapses. It satisfies the Clock
// interface so the Timer can run against real time.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// runTimerFn is the seam for the main execution logic. The production
// implementation instantiates a Timer with a real Clock and its output
// writer, runs it, and handles SIGINT by delegating to handleSIGINTFn. Tests
// replace this seam with a capturing stub (see withRunTimer), so this body
// only runs in the real program.
var runTimerFn = func(ctx context.Context, focus, short, long time.Duration) error {
	// Derive a cancellable context so a SIGINT can stop the countdown even
	// though the caller hands us a non-cancellable one (context.Background).
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	clk := realClock{}
	out := os.Stdout

	t := NewTimer(focus, short, long, clk, out)

	// Listen for SIGINT on a buffered channel so a signal is never dropped
	// before the select below is ready.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	// Cancel the countdown when the interrupt arrives so Timer.Run returns
	// promptly.
	go func() {
		sig := <-sigCh
		if sig != nil {
			cancel()
		}
	}()

	// Run the countdown loop; it returns when the context is cancelled.
	if err := t.Run(ctx); err != nil {
		return err
	}

	// A SIGINT interrupted the run: print the completion line and exit 0.
	select {
	case <-sigCh:
		code, err := handleSIGINTFn(t, ctx, out)
		osExit(code)
		return err
	default:
	}
	return nil
}
