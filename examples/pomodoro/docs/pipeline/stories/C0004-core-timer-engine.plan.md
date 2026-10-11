# Core Timer Engine — story plan

> **For agentic workers:** execute wave by wave; qa writes tests, coder implements.

**Goal:** Implement a lightweight terminal-based Pomodoro timer with a 4-session cycle, ANSI in-pass updates, and SIGINT handling.
**Architecture:** A core 'Timer' engine manages the state machine and countdown logic, decoupled from time (Clock interface) and output (io.Writer). A main entry point wires flags, signals, and the engine.
**Tech Stack:** Go, standard library only
**Spec:** docs/pipeline/stories/C0004-core-timer-engine.contract.md    **Story Contract:** docs/pipeline/stories/C0004-core-timer-engine.contract.json
**Context:** tools
**Lint:** `ai-mode plan lint` exit 0 · **Verify:** `ai-mode plan verify --task all` exit 0

_Rendered by `ai-mode plan render` from the plan JSON next to this file; change the JSON, not this file._

## Global Constraints
- Language: Go (standard library only).
- Platform: macOS Terminal.
- No external dependencies or network/disk access.
- Maximum session length must not exceed 24 hours to prevent resource exhaustion.

## Review Focus
1. SIGINT handling during different states (S3, S6, S9)
2. The 4-session cycle transition logic (S1, S2, S16)
3. ANSI terminal output formatting (S5, S7, S8)
4. Input validation for durations > 24h (S4, S12, S13)
5. The 'Completed N' count accuracy (S2, S21, S22)

## Boundaries
| Boundary | Contract artifact | Fixtures | Fake | Provider check | Owner task |
| --- | --- | --- | --- | --- | --- |
| time | clock.go (Clock interface) | none | fakeClock in clock_test.go | timer_test.go | T01 |
| terminal output | io.Writer | none | bytes.Buffer in timer_test.go | timer_test.go | T02 |

## Task graph

```json
{
 "hot_files": [
  "go.mod",
  "README.md"
 ],
 "tasks": [
  {
   "id": "T01",
   "title": "Contract: shared types and clock seam",
   "kind": "contract",
   "packet": "code",
   "depends_on": [],
   "owns": [
    "timer_types.go",
    "clock_test.go"
   ],
   "produces": [
    "type Clock interface",
    "type Timer struct",
    "var runTimerFn"
   ],
   "consumes": [],
   "covers": [],
   "test_level": "contract",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T02",
   "title": "Implementation: Timer engine logic",
   "kind": "impl",
   "packet": "spec",
   "depends_on": [
    "T01"
   ],
   "owns": [
    "timer.go",
    "timer_test.go"
   ],
   "produces": [
    "func NewTimer(...)",
    "func (t *Timer) Run(...)"
   ],
   "consumes": [
    "Clock",
    "TimerState"
   ],
   "covers": [
    "S1",
    "S2",
    "S5",
    "S7",
    "S8",
    "S14",
    "S16",
    "S18"
   ],
   "test_level": "unit",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T03",
   "title": "Integration: CLI and Signal handling",
   "kind": "integration",
   "packet": "code",
   "depends_on": [
    "T01",
    "T02"
   ],
   "owns": [
    "main.go",
    "main_test.go"
   ],
   "produces": [
    "func main()"
   ],
   "consumes": [
    "runTimerFn"
   ],
   "covers": [
    "S3",
    "S4",
    "S6",
    "S9",
    "S10",
    "S11",
    "S12",
    "S13",
    "S15",
    "S17",
    "S19",
    "S20",
    "S21",
    "S22"
   ],
   "test_level": "",
   "stack_layer": 1,
   "branch": ""
  }
 ]
}
```

**Scenario coverage**

| Scenario | Rule | Task | Test |
| --- | --- | --- | --- |
| S1 | R1 | T02 | `TestTimer_S1_ShortBreakAfterFocus` |
| S2 | R1 | T02 | `TestTimer_S2_FullCycle` |
| S3 | R1 | T03 | `TestMain_S3_SIGINTDuringLongBreak` |
| S4 | R1 | T03 | `TestMain_S4_DurationTooLong` |
| S5 | R1 | T02 | `TestTimer_S5_ANSIUpdates` |
| S6 | R1 | T03 | `TestMain_S6_SIGINTDuringFocus` |
| S7 | R2 | T02 | `TestTimer_S7_StatusUpdate` |
| S8 | R2 | T02 | `TestTimer_S8_LongBreakLabel` |
| S9 | R3 | T03 | `TestMain_S9_SIGINTClearsLine` |
| S10 | R3 | T03 | `TestMain_S10_SIGINTDuringLongBreak` |
| S11 | R3 | T03 | `TestMain_S11_SIGINTImmediately` |
| S12 | R4 | T03 | `TestMain_S12_FlagValidation` |
| S13 | R4 | T03 | `TestMain_S13_MultipleViolations` |
| S14 | R2 | T02 | `TestTimer_S14_BoundaryOneMin` |
| S15 | R2 | T03 | `TestMain_S15_ZeroDuration` |
| S16 | R5 | T02 | `TestTimer_S16_CycleRestart` |
| S17 | R6 | T03 | `TestMain_S17_SIGINTDuringFocus` |
| S18 | R2 | T02 | `TestTimer_S18_InitialOutput` |
| S19 | R7 | T03 | `TestMain_S19_FlagValidation` |
| S20 | R8 | T03 | `TestMain_S20_SIGINTInSecondCycle` |
| S21 | R8 | T03 | `TestMain_S21_InterruptedFirstSession` |
| S22 | R8 | T03 | `TestMain_S22_InterruptedBeforeEnd` |

## Task packets

### T01: Contract: shared types and clock seam

**Kind / packet / stack layer / test level / depends on:** contract / code / 1 / contract / none
**Owns (exclusive):** `timer_types.go`, `clock_test.go`
**Covers:** none
**Spec excerpt:** none

**Interfaces**
- Consumes: nothing
- Produces: type Clock interface; type Timer struct; var runTimerFn

**Phase A: the contract code.**

```go file=timer_types.go
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
```

```go file=clock_test.go
package main

import (
	"time"
)

type FakeClock struct {
	now time.Time
}

func (f *FakeClock) Now() time.Time { return f.now }
func (f *FakeClock) After(d time.Duration) <-chan time.Time {
	// In a real fake, this would trigger after d
	return make(<-chan time.Time)
}
```

Run: `go vet ./...`

### T02: Implementation: Timer engine logic

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `timer.go`, `timer_test.go`
**Covers:** S1, S2, S5, S7, S8, S14, S16, S18
**Spec excerpt:** "The timer must transition between focus sessions, short breaks, and long breaks according to the four-session cycle logic"

**Interfaces**
- Consumes: Clock; TimerState
- Produces: func NewTimer(...); func (t *Timer) Run(...)

Implement NewTimer and Run. Run must loop, check clock.After, and update state. Use the provided Clock and io.Writer. Use the ANSI sequence \r\x1b[K for updates.

**Scenarios (write these tests first):**
- S1 `TestTimer_S1_ShortBreakAfterFocus`: After 4 focus sessions, next transition is LongBreak
- S2 `TestTimer_S2_FullCycle`: Transition sequence: Focus -> ShortBreak -> Focus -> LongBreak
- S5 `TestTimer_S5_ANSIUpdates`: Timer writes 'Session 1: 25:00\r\x1b[K' to writer
- S7 `TestTimer_S7_StatusUpdate`: Writer receives correct countdown strings
- S8 `TestTimer_S8_LongBreakLabel`: Long break uses 'Long break: MM:SS' label
- S14 `TestTimer_S14_BoundaryOneMin`: Transitions correctly at exactly 1 minute
- S16 `TestTimer_S16_CycleRestart`: After long break, cycle resets to focus session 5
- S18 `TestTimer_S18_InitialOutput`: Starts with 'Session 1: 25:00\r\x1b[K'

Run: `go test ./... -count=1 -run '^(TestTimer_S1_ShortBreakAfterFocus|TestTimer_S2_FullCycle|TestTimer_S5_ANSIUpdates|TestTimer_S7_StatusUpdate|TestTimer_S8_LongBreakLabel|TestTimer_S14_BoundaryOneMin|TestTimer_S16_CycleRestart|TestTimer_S18_InitialOutput)$'`

### T03: Integration: CLI and Signal handling

**Kind / packet / stack layer / test level / depends on:** integration / code / 1 / unit / T01, T02
**Owns (exclusive):** `main.go`, `main_test.go`
**Covers:** S3, S4, S6, S9, S10, S11, S12, S13, S15, S17, S19, S20, S21, S22
**Spec excerpt:** "On SIGINT, the program must clear the current in-place status line, print exactly 'Completed N focus sessions\n' ... and exit with status 0"

**Interfaces**
- Consumes: runTimerFn
- Produces: func main()

Implement the main function: parse flags, validate (must be <= 24h and > 0), setup signal.NotifyContext, and call runTimerFn. The integration test should use a sub-process or a controlled environment to send SIGINT.

**Scenarios (write these tests first):**
- S3 `TestMain_S3_SIGINTDuringLongBreak`: SIGINT during long break prints 'Completed 4 focus sessions'
- S4 `TestMain_S4_DurationTooLong`: Fails if duration > 24h
- S6 `TestMain_S6_SIGINTDuringFocus`: SIGINT during focus session does not count that session
- S9 `TestMain_S9_SIGINTClearsLine`: SIGINT clears existing line with \r\x1b[K
- S10 `TestMain_S10_SIGINTDuringLongBreak`: SIGINT during long break counts completed sessions
- S11 `TestMain_S11_SIGINTImmediately`: SIGINT at start prints 'Completed 0 focus sessions'
- S12 `TestMain_S12_FlagValidation`: Flag --focus 25h prints error and exits 1
- S13 `TestMain_S13_MultipleViolations`: Reports error if multiple durations exceed 24h
- S15 `TestMain_S15_ZeroDuration`: Handles zero duration as configured
- S17 `TestMain_S17_SIGINTDuringFocus`: SIGINT during focus session counts completed sessions
- S19 `TestMain_S19_FlagValidation`: Flag --focus 25h prints error and exits 1
- S20 `TestMain_S20_SIGINTInSecondCycle`: SIGINT in focus session of second cycle counts all previous completions
- S21 `TestMain_S21_InterruptedFirstSession`: Interrupted first session results in 0 completed
- S22 `TestMain_S22_InterruptedBeforeEnd`: Interrupted session 4 one second before end does not count it

**Phase A: tests plus stubs (role: qa).**

```go file=main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	focus := flag.Duration("focus", 25*time.Minute, "focus session duration")
	short := flag.Duration("short", 5*time.Minute, "short break duration")
	long := flag.Duration("long", 15*time.Minute, "long break duration")
	flag.Parse()

	if *focus > 24*time.Hour || *short > 24*time.Hour || *long > 24*time.Hour {
		fmt.Fprintf(os.Stderr, "error: duration exceeds 24h limit\n")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err := runTimerFn(ctx, *focus, *short, *long)
	if err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
```

Run: `go test ./... -count=1 -run '^(TestMain_S3_SIGINTDuringLongBreak|TestMain_S4_DurationTooLong|TestMain_S6_SIGINTDuringFocus|TestMain_S9_SIGINTClearsLine|TestMain_S10_SIGINTDuringLongBreak|TestMain_S11_SIGINTImmediately|TestMain_S12_FlagValidation|TestMain_S13_MultipleViolations|TestMain_S15_ZeroDuration|TestMain_S17_SIGINTDuringFocus|TestMain_S19_FlagValidation|TestMain_S20_SIGINTInSecondCycle|TestMain_S21_InterruptedFirstSession|TestMain_S22_InterruptedBeforeEnd)$'`
