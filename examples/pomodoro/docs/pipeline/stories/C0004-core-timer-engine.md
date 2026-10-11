# Core Timer Engine

- **Card:** C0004 · **Plan:** C0003 (docs/pipeline/plans/C0002-pomodoro-cli-feature-plan.md, story ST1) · **Spec:** docs/pipeline/specs/C0002-pomodoro-cli.md
- **Depends on:** none · **Size:** M · **Flag:** none

## Story

As a macOS user, I want a lightweight terminal-based Pomodoro timer, so that I can manage focus sessions and breaks without distractions.

## Scope

Implement the basic timer loop, session transitions (Short/Long break logic), and terminal output using carriage returns.

## Scenarios (from the spec)

- **S1 — Standard Cycle.** Given default durations, when 25m focus and 5m short break pass, then the terminal shows "Session 1: 25:00" → "Session 1: 00:00" → "Break: 05:00".
- **S2 — Long Break Trigger.** Given default durations, when 4 focus sessions and 3 short breaks complete, then the 4th break must be 15 minutes long.
- **S4 — Graceful Exit.** Given a running timer, when `SIGINT` is received, then the program prints the completed count and exits without a stack trace.

## Global Constraints

- Language: Go (standard library only).
- Platform: macOS Terminal.
- No external dependencies or network/disk access.
- Maximum session length must not exceed 24 hours to prevent resource exhaustion.

## Contracts (from the feature plan)

The seams the stories share, decided now so no story depends on another story's internals:
- `main.go` (package `main`) will contain the core logic.
- Interfaces for `Clock` and `Writer` will be defined in `main.go` to allow dependency injection for testing.
- The logic will be encapsulated in a `Timer` struct that manages the state machine and countdown.

```go
package main

import (
	"io"
	"time"
)

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
	// internal state tracking
}

// Run starts the timer loop and handles transitions.
func (t *Timer) Run() error {
	return nil
}
```

- The test suite will use a `FakeClock` implementation of `Clock` and a `bytes.Buffer` as the `io.Writer`.
