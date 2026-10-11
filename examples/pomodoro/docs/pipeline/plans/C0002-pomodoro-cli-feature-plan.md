# Pomodoro CLI — feature plan

**Spec:** docs/pipeline/specs/C0002-pomodoro-cli.md
**Card:** C0002

## Contracts
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

## Stories

```json
{"stories":[
 {"id":"ST1","title":"Core Timer Engine","scope":"Implement the basic timer loop, session transitions (Short/Long break logic), and terminal output using carriage returns.","scenarios":["S1","S2","S4"],"depends_on":[],"size":"M"},
 {"id":"ST2","title":"CLI Configuration & Validation","scope":"Add flag parsing for custom durations and implement input validation (negative/large values).","scenarios":["S3","S5"],"depends_on":["ST1"],"size":"S"}
],
"flags":[]}
```

## Story notes
### ST1: Core Timer Engine
This story establishes the heart of the application. It implements the state machine (Focus -> Break), the ticker logic, the notification bell, and the graceful exit handling. It is deliverable on its own because the timer functions correctly with defaults and handles interruptions safely, even if it doesn't yet support custom flags.

### ST2: CLI Configuration & Validation
This story extends the engine with user-controlled parameters. It's a logical second step that adds the complexity of `flag` parsing and the business rules for valid input durations. It depends on the engine being able to consume these durations.
