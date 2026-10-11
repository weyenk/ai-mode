# Core Timer Engine — story plan

> **For agentic workers:** execute wave by wave; qa writes tests, coder implements.

**Goal:** Implement a lightweight terminal-based Pomodoro timer with session management and signal handling.
**Architecture:** The engine (Timer.Run) manages state transitions and countdowns via a Clock interface; the CLI (main.go) handles configuration, validation, and SIGINT cleanup.
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
1. The 4-session cycle logic and long break trigger (S1, S2, S16)
2. SIGINT signal handling and the 'Completed N' count (S3, S6, S9, S22)
3. Duration validation for the 25h/24h upper bound and 1s lower bound (S4, S12, S13, S15)
4. ANSI escape sequence formatting for in-place updates (S5, S7, S8)
5. The requirement that interrupted sessions are not counted (S6, S21, S22)

## Boundaries
| Boundary | Contract artifact | Fixtures | Fake | Provider check | Owner task |
| --- | --- | --- | --- | --- | --- |
| time | Clock interface | none | fakeClock with manual Advance() | timer_test.go | T01 |
| terminal output | io.Writer | none | bytes.Buffer | timer_test.go | T02 |

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
   "title": "Contract: shared types",
   "kind": "contract",
   "packet": "code",
   "depends_on": [],
   "owns": [
    "main.go"
   ],
   "produces": [
    "type Clock interface",
    "type TimerState",
    "type Timer"
   ],
   "consumes": [],
   "covers": [],
   "test_level": "contract",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T02",
   "title": "Engine: Timer logic and transitions",
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
    "Timer.Run implementation"
   ],
   "consumes": [
    "Clock",
    "TimerState",
    "Timer"
   ],
   "covers": [
    "S1",
    "S2",
    "S5",
    "S7",
    "S8",
    "S14",
    "S15",
    "S16",
    "S18"
   ],
   "test_level": "unit",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T03",
   "title": "Integration: CLI and Signal Handling",
   "kind": "integration",
   "packet": "spec",
   "depends_on": [
    "T01",
    "T02"
   ],
   "owns": [
    "main.go",
    "main_test.go"
   ],
   "produces": [
    "main() function",
    "flag parsing and validation"
   ],
   "consumes": [
    "Timer",
    "Clock"
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
    "S17",
    "S19",
    "S20",
    "S21",
    "S22"
   ],
   "test_level": "functional",
   "stack_layer": 1,
   "branch": ""
  }
 ]
}
```

**Scenario coverage**

| Scenario | Rule | Task | Test |
| --- | --- | --- | --- |
| S1 | R1 | T02 | `TestTimer_S1_ShortBreakAfterFourth` |
| S2 | R1 | T02 | `TestTimer_S2_FullCycle` |
| S3 | R1 | T03 | `TestMain_S3_SIGINTInLongBreak` |
| S4 | R1 | T03 | `TestMain_S4_DurationLimitExceeded` |
| S5 | R1 | T02 | `TestTimer_S5_OutputFormat` |
| S6 | R1 | T03 | `TestMain_S6_SIGINTInFocus` |
| S7 | R2 | T02 | `TestTimer_S7_BreakTransitionOutput` |
| S8 | R2 | T02 | `TestTimer_S8_LongBreakLabel` |
| S9 | R3 | T03 | `TestMain_S9_SIGINTClearLine` |
| S10 | R3 | T03 | `TestMain_S10_SIGINTInLongBreakAfterFour` |
| S11 | R3 | T03 | `TestMain_S11_SIGINTImmediately` |
| S12 | R4 | T03 | `TestMain_S12_FlagError` |
| S13 | R4 | T03 | `TestMain_S13_MultipleFlagErrors` |
| S14 | R2 | T02 | `TestTimer_S14_BoundaryOneMinute` |
| S15 | R2 | T02 | `TestTimer_S15_ZeroDuration` |
| S16 | R5 | T02 | `TestTimer_S16_CycleRestart` |
| S17 | R6 | T03 | `TestMain_S17_SIGINTFocusInterrupted` |
| S18 | R2 | T02 | `TestTimer_S18_InitialOutput` |
| S19 | R7 | T03 | `TestMain_S19_FlagLimit` |
| S20 | R8 | T03 | `TestMain_S20_SIGINTCycleRestart` |
| S21 | R8 | T03 | `TestMain_S21_SIGINTStart` |
| S22 | R8 | T03 | `TestMain_S22_SIGINTEndBoundary` |

## Task packets

### T01: Contract: shared types

**Kind / packet / stack layer / test level / depends on:** contract / code / 1 / contract / none
**Owns (exclusive):** `main.go`
**Covers:** none
**Spec excerpt:** none

**Interfaces**
- Consumes: nothing
- Produces: type Clock interface; type TimerState; type Timer

**Phase A: the contract code.**

```go file=main.go
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
	currentSession int // Session number within the 4-session cycle (1-4)
}

// Run starts the timer loop and handles transitions.
func (t *Timer) Run(ctx context.Context) error {
	return nil
}
```

Run: `go vet ./...`

### T02: Engine: Timer logic and transitions

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `timer.go`, `timer_test.go`
**Covers:** S1, S2, S5, S7, S8, S14, S15, S16, S18
**Spec excerpt:** "R1 The timer must transition between focus sessions, short breaks, and long breaks according to the four-session cycle logic"

**Interfaces**
- Consumes: Clock; TimerState; Timer
- Produces: Timer.Run implementation

Implementation of Run() must use a Ticker or Clock.After. It must track elapsed time and update the Writer every second. It must increment completedFocus only when a focus session finishes.

**Scenarios (write these tests first):**
- S1 `TestTimer_S1_ShortBreakAfterFourth`: Given 4 completed focus sessions, the next transition from Focus to Break is a LongBreak
- S2 `TestTimer_S2_FullCycle`: Verifies the complete sequence of Focus -> ShortBreak -> Focus -> LongBreak
- S5 `TestTimer_S5_OutputFormat`: Ensures output uses \r\x1b[K and correct labels like 'Session 1: 25:00'
- S7 `TestTimer_S7_BreakTransitionOutput`: Verifies output switches from 'Session X' to 'Break: MM:SS'
- S8 `TestTimer_S8_LongBreakLabel`: Verifies LongBreak uses 'Long break: MM:SS' label
- S14 `TestTimer_S14_BoundaryOneMinute`: Timer works correctly with 1-minute durations without skipping 00:00
- S15 `TestTimer_S15_ZeroDuration`: Ensure 0 or negative durations are handled (per R15/R15 ambiguity)
- S16 `TestTimer_S16_CycleRestart`: After a long break, the next session is Focus Session 5 (resetting cycle count)
- S18 `TestTimer_S18_InitialOutput`: Immediately displays correct session and duration

Run: `go test ./... -count=1 -run '^(TestTimer_S1_ShortBreakAfterFourth|TestTimer_S2_FullCycle|TestTimer_S5_OutputFormat|TestTimer_S7_BreakTransitionOutput|TestTimer_S8_LongBreakLabel|TestTimer_S14_BoundaryOneMinute|TestTimer_S15_ZeroDuration|TestTimer_S16_CycleRestart|TestTimer_S18_InitialOutput)$'`

### T03: Integration: CLI and Signal Handling

**Kind / packet / stack layer / test level / depends on:** integration / spec / 1 / functional / T01, T02
**Owns (exclusive):** `main.go`, `main_test.go`
**Covers:** S3, S4, S6, S9, S10, S11, S12, S13, S17, S19, S20, S21, S22
**Spec excerpt:** "R3 On SIGINT, the program must clear the current in-place status line, print exactly 'Completed N focus sessions\n'..."

**Interfaces**
- Consumes: Timer; Clock
- Produces: main() function; flag parsing and validation

Main must use signal.NotifyContext to catch SIGINT. It must parse flags, validate durations (1s <= d <= 24h), instantiate Timer, and call Run. On context cancellation, it must print the final message using \r\x1b[K.

**Scenarios (write these tests first):**
- S3 `TestMain_S3_SIGINTInLongBreak`: SIGINT during long break prints 'Completed 4 focus sessions' and exits 0
- S4 `TestMain_S4_DurationLimitExceeded`: Configuring 25h duration fails startup with error
- S6 `TestMain_S6_SIGINTInFocus`: SIGINT during focus session does NOT count the interrupted session
- S9 `TestMain_S9_SIGINTClearLine`: SIGINT prints \r\x1b[K to clear remnants before printing completion message
- S10 `TestMain_S10_SIGINTInLongBreakAfterFour`: SIGINT during long break after 4 sessions shows 'Completed 4 focus sessions'
- S11 `TestMain_S11_SIGINTImmediately`: SIGINT at startup shows 'Completed 0 focus sessions'
- S12 `TestMain_S12_FlagError`: Passing --focus 25h prints error to stderr and exits 1
- S13 `TestMain_S13_MultipleFlagErrors`: Reporting multiple duration violations
- S17 `TestMain_S17_SIGINTFocusInterrupted`: SIGINT during focus session 3 shows 'Completed 2 sessions'
- S19 `TestMain_S19_FlagLimit`: Direct check of flag parsing for 25h error
- S20 `TestMain_S20_SIGINTCycleRestart`: SIGINT during session 5 counts previous cycle's 4 sessions
- S21 `TestMain_S21_SIGINTStart`: SIGINT at t=1s counts 0 completed sessions
- S22 `TestMain_S22_SIGINTEndBoundary`: SIGINT 1s before end of session does not count it

Run: `go test ./... -count=1 -run '^(TestMain_S3_SIGINTInLongBreak|TestMain_S4_DurationLimitExceeded|TestMain_S6_SIGINTInFocus|TestMain_S9_SIGINTClearLine|TestMain_S10_SIGINTInLongBreakAfterFour|TestMain_S11_SIGINTImmediately|TestMain_S12_FlagError|TestMain_S13_MultipleFlagErrors|TestMain_S17_SIGINTFocusInterrupted|TestMain_S19_FlagLimit|TestMain_S20_SIGINTCycleRestart|TestMain_S21_SIGINTStart|TestMain_S22_SIGINTEndBoundary)$'`
