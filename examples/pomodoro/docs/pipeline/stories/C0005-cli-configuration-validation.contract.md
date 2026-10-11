# Story Contract: CLI Configuration & Validation

- **Story:** docs/pipeline/stories/C0005-cli-configuration-validation.md · **Verdict:** ready (product, qa, architect) · **Meeting:** 20261011-232838-2e8d92 · **Flag:** none

## Story

As a macOS user, I want to configure custom focus and break durations via command-line flags, so that I can tailor my Pomodoro sessions to my workflow without distractions.

## Rules

- R1 The program must validate all three durations (-focus, -short, -long) to ensure they are whole minutes, at least 1 minute, and no more than 1440 minutes (24 hours). If any value is invalid, the program must print an error message naming the flag and exit with status 2 before starting any session.
- R2 The program must validate all three durations (-focus, -short, -long) to be between 1 and 1440 minutes, exiting with status 2 and a specific error message upon encountering the first invalid flag.
- R3 Validation must check -focus, then -short, then -long in that fixed order and stop at the first out-of-range value, exiting with status 2 and a message naming that flag.
- R4 Valid durations (1–1440 whole minutes) must be applied to the corresponding Timer field before any session countdown begins; defaults apply only to flags that were not supplied.

## Scenarios

Each scenario is one example from the Story Contract; S<n> is example n. Tests carry the id in their name.

- **S1** (R1) Given flags `-focus 50 -short 10`, when the timer starts, then the first session lasts 50 minutes.
- **S2** (R1) Given `-focus -1`, when the program starts, then it prints an error and exits.
- **S3** (R1) Given `-focus 0`, when the program starts, then it prints 'Invalid value for -focus: must be between 1 and 1440 minutes.' and exits with status 2.
- **S4** (R1) Given `-focus 1441`, when the program starts, then it prints 'Invalid value for -focus: must be between 1 and 1440 minutes.' and exits with status 2.
- **S5** (R1) Given `-focus 1440`, when the timer starts, then the first session lasts 1440 minutes (24 hours).
- **S6** (R1) Given `-short 1`, when the timer starts, then the short break lasts 1 minute.
- **S7** (R1) Given `-long 1441`, when the program starts, then it prints 'Invalid value for -long: must be between 1 and 1440 minutes.' and exits with status 2.
- **S8** (R2) Given `-short 1441`, when the program starts, then it prints 'Invalid value for -short: must be between 1 and 1440 minutes.' and exits with status 2.
- **S9** (R3) Given `-focus 200 -short -1 -long 5`, when the program starts, it prints 'Invalid value for -focus: must be between 1 and 1440 minutes.' and exits 2, never mentioning -short.
- **S10** (R3) Given `-focus 0 -short -1 -long 5`, the first error is for -focus (0), not -short, and exit status is 2.
- **S11** (R4) Given no flags, the Timer is constructed with focusDuration=25*time.Minute, shortDuration=5*time.Minute, longDuration=15*time.Minute, and the first countdown uses 25 minutes.
- **S12** (R4) Given `-focus 50` only, focusDuration=50*time.Minute, shortDuration stays 5*time.Minute, longDuration stays 15*time.Minute.

## Layer plan

| Layer | Name | Scenarios | Test level |
| --- | --- | --- | --- |
| 1 | CLI implementation | S1, S2, S3, S4, S5, S6, S7, S8, S9, S10, S11, S12 | functional |

## Gates

- static-analysis: active
- mutation: active
- performance: not-available
- ai-evals: not-available
- dynamic-security: not-applicable

## Resolved questions

- What is the exact format of the error message that should be printed for invalid inputs? → The error message should include the name of the flag and a description of the issue, such as 'Invalid value for -focus: must be between 1 and 1440 minutes.'
- How will the program handle cases where multiple flags are invalid at once? → The program should validate flags in a defined order (-focus, -short, -long) and report the first invalid flag it encounters. It should exit immediately with status 2 without checking the remaining flags.
- Are non-integer inputs like `-focus 2.5` expected to be rejected by validation with the same status-2 error, or should they be handled as a flag-parsing failure before the 1–1440 check runs? → It will be handled as a flag-parsing failure by the Go `flag` package if we use `flag.Int`. This satisfies the requirement to exit with status 2 and name the flag, avoiding the need for the extra complexity of manual string-to-int parsing logic.
- The exact error message format for values below 1 → To ensure consistency with the existing test examples and simplify the validation logic, we will use a unified error message for any value outside the allowed range (both < 1 and > 1440): 'Invalid value for [flag]: must be between 1 and 1440 minutes.'

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
