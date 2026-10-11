# CLI Configuration & Validation

- **Card:** C0005 · **Plan:** C0003 (docs/pipeline/plans/C0002-pomodoro-cli-feature-plan.md, story ST2) · **Spec:** docs/pipeline/specs/C0002-pomodoro-cli.md
- **Depends on:** C0004 · **Size:** S · **Flag:** none

## Story

As a macOS user, I want to configure custom focus and break durations via command-line flags, so that I can tailor my Pomodoro sessions to my workflow without distractions.

## Scope

Add flag parsing for custom durations and implement input validation (negative/large values).

## Scenarios (from the spec)

- **S3 — Custom Configuration.** Given flags `-focus 50 -short 10`, when the timer starts, then the first session lasts 50 minutes.
- **S5 — Invalid Input.** Given `-focus -1`, when the program starts, then it prints an error and exits.

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

## Answers from the requester

- All three lengths (-focus, -short, -long) follow the same rules: whole minutes, at least 1 and at most 1440 (24 hours); anything else prints an error naming the flag and exits with status 2 before any session starts. Defaults when a flag is not given: -focus 25, -short 5, -long 15. (requester (answered by the operator, Claude, from the original request), 2026-10-11)
