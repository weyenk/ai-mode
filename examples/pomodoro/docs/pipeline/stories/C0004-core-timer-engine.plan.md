# Core Timer Engine — story plan

> **For agentic workers:** execute wave by wave; qa writes tests, coder implements.

**Goal:** Implement the core Pomodoro timer engine with state transitions, in-place terminal updates, and SIGINT handling.
**Architecture:** The engine is centered around the `Timer` struct in `timer.go`, using `Clock` and `io.Writer` interfaces for testability. Transitions and updates are driven by a loop in `Timer.Run` that reacts to clock ticks and context cancellation.
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
1. SIGINT handling precisely clearing the line (S9, S10, S11)
2. Correct session counting (interrupted sessions not counted) (S6, S22)
3. Correct transition logic for the 4-session cycle (S1, S2, S16)
4. ANSI escape sequence accuracy for in-place updates (S5, S7, S14)
5. Validation of durations at startup (S4, S12, S13, S19)

## Boundaries
| Boundary | Contract artifact | Fixtures | Fake | Provider check | Owner task |
| --- | --- | --- | --- | --- | --- |
| time | clock_test.go (testClock) | none | testClock provides manual advancement of time | timer_test.go | T01 |
| terminal output | output.go (updateOutputFn) | none | notifyWriter records all writes to a slice for assertion | output_test.go | T03 |
| process signals | interrupt.go (handleSIGINTFn) | none | bytes.Buffer used to capture output in unit tests | interrupt_test.go | T04 |

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
    "clock_test.go",
    "timer_types.go"
   ],
   "produces": [
    "type Clock interface",
    "type TimerState int",
    "type Timer struct"
   ],
   "consumes": [],
   "covers": [],
   "test_level": "contract",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T02",
   "title": "Timer Loop and Transitions",
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
    "func (t *Timer) Run(ctx context.Context) error",
    "func (t *Timer) transition(state *TimerState)",
    "func (t *Timer) durationFor(state TimerState) time.Duration"
   ],
   "consumes": [
    "Clock"
   ],
   "covers": [
    "S1",
    "S2",
    "S14",
    "S16"
   ],
   "test_level": "unit",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T03",
   "title": "Output Formatting",
   "kind": "impl",
   "packet": "spec",
   "depends_on": [
    "T01"
   ],
   "owns": [
    "output.go",
    "output_test.go"
   ],
   "produces": [
    "var updateOutputFn"
   ],
   "consumes": [
    "TimerState"
   ],
   "covers": [
    "S5",
    "S7",
    "S8",
    "S18"
   ],
   "test_level": "unit",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T04",
   "title": "Interrupt Handling",
   "kind": "impl",
   "packet": "spec",
   "depends_on": [
    "T01"
   ],
   "owns": [
    "interrupt.go",
    "interrupt_test.go"
   ],
   "produces": [
    "func handleSIGINTFn"
   ],
   "consumes": [
    "Timer"
   ],
   "covers": [
    "S6",
    "S21",
    "S22"
   ],
   "test_level": "unit",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T05",
   "title": "Completion Message Logic",
   "kind": "impl",
   "packet": "spec",
   "depends_on": [
    "T01"
   ],
   "owns": [
    "completion.go",
    "completion_test.go"
   ],
   "produces": [
    "var runCompletion"
   ],
   "consumes": [
    "Timer"
   ],
   "covers": [
    "S3",
    "S9",
    "S10",
    "S11",
    "S17",
    "S20"
   ],
   "test_level": "unit",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T06",
   "title": "Flag Validation",
   "kind": "impl",
   "packet": "spec",
   "depends_on": [
    "T01"
   ],
   "owns": [
    "flags.go",
    "flags_test.go"
   ],
   "produces": [
    "var validateFlagsFn"
   ],
   "consumes": [],
   "covers": [
    "S4",
    "S12",
    "S13",
    "S15",
    "S19"
   ],
   "test_level": "unit",
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
| S3 | R1 | T05 | `TestCompletion_S3_LongBreak` |
| S4 | R1 | T06 | `TestFlags_S4_TooLong` |
| S5 | R1 | T03 | `TestOutput_S5_Formatting` |
| S6 | R1 | T04 | `TestInterrupt_S6_FocusInterrupted` |
| S7 | R2 | T03 | `TestOutput_S7_ShortBreak` |
| S8 | R2 | T03 | `TestOutput_S8_LongBreak` |
| S9 | R3 | T05 | `TestCompletion_S9_ClearLine` |
| S10 | R3 | T05 | `TestCompletion_S10_AfterSession4` |
| S11 | R3 | T05 | `TestCompletion_S11_Start` |
| S12 | R4 | T06 | `TestFlags_S12_ErrorMsg` |
| S13 | R4 | T06 | `TestFlags_S13_MultipleErrors` |
| S14 | R2 | T02 | `TestTimer_S14_BoundaryOneMinute` |
| S15 | R2 | T06 | `TestFlags_S15_NonPositive` |
| S16 | R5 | T02 | `TestTimer_S16_CycleRestart` |
| S17 | R6 | T05 | `TestCompletion_S17_FocusInterrupted` |
| S18 | R2 | T03 | `TestOutput_S18_Initial` |
| S19 | R7 | T06 | `TestFlags_S19_DuplicateCheck` |
| S20 | R8 | T05 | `TestCompletion_S20_Session5Start` |
| S21 | R8 | T04 | `TestInterrupt_S21_StartInterrupted` |
| S22 | R8 | T04 | `TestInterrupt_S22_EndOfSessionInterrupted` |

## Task packets

### T01: Contract: shared types

**Kind / packet / stack layer / test level / depends on:** contract / code / 1 / contract / none
**Owns (exclusive):** `clock_test.go`, `timer_types.go`
**Covers:** none
**Spec excerpt:** none

**Interfaces**
- Consumes: nothing
- Produces: type Clock interface; type TimerState int; type Timer struct

**Phase A: the contract code.**

```go file=timer_types.go
package main

import (
	"context"
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
}
```

Run: `go vet ./...`

### T02: Timer Loop and Transitions

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `timer.go`, `timer_test.go`
**Covers:** S1, S2, S14, S16
**Spec excerpt:** "The timer must transition between focus sessions, short breaks, and long breaks according to the four-session cycle logic" and "The timer must update the in-place status line every second using carriage return and ANSI erase-to-end-of-line"

**Interfaces**
- Consumes: Clock
- Produces: func (t *Timer) Run(ctx context.Context) error; func (t *Timer) transition(state *TimerState); func (t *Timer) durationFor(state TimerState) time.Duration

Implement the Run loop using a select on Clock.After and context.Done. Ensure transition logic increments completedFocus and handles the 4-session logic (modulo 4).

**Scenarios (write these tests first):**
- S1 `TestTimer_S1_ShortBreakAfterFourth`: After 4 focus sessions, the next break is a long break.
- S2 `TestTimer_S2_FullCycle`: Timer transitions correctly through Focus, ShortBreak, and LongBreak in the 4-session cycle.
- S14 `TestTimer_S14_BoundaryOneMinute`: Timer handles 1-minute durations correctly without negative countdowns.
- S16 `TestTimer_S16_CycleRestart`: After a long break, the cycle restarts at session 5.

Run: `go test ./... -count=1 -run '^(TestTimer_S1_ShortBreakAfterFourth|TestTimer_S2_FullCycle|TestTimer_S14_BoundaryOneMinute|TestTimer_S16_CycleRestart)$'`

### T03: Output Formatting

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `output.go`, `output_test.go`
**Covers:** S5, S7, S8, S18
**Spec excerpt:** "the writer receives 'Session 1: 25:00\r\x1b[K'" and "the writer receives 'Break: 05:00\r\x1b[K'" and "the writer receives 'Long break: 15:00\r\x1b[K'"

**Interfaces**
- Consumes: TimerState
- Produces: var updateOutputFn

Implement updateOutputFn to format the duration as MM:SS and include the ANSI escape sequence.

**Scenarios (write these tests first):**
- S5 `TestOutput_S5_Formatting`: Output format matches 'Session N: MM:SS\r\x1b[K'.
- S7 `TestOutput_S7_ShortBreak`: Transitions between countdowns correctly.
- S8 `TestOutput_S8_LongBreak`: Long break label is 'Long break'.
- S18 `TestOutput_S18_Initial`: Initial output is formatted correctly.

Run: `go test ./... -count=1 -run '^(TestOutput_S5_Formatting|TestOutput_S7_ShortBreak|TestOutput_S8_LongBreak|TestOutput_S18_Initial)$'`

### T04: Interrupt Handling

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `interrupt.go`, `interrupt_test.go`
**Covers:** S6, S21, S22
**Spec excerpt:** "On SIGINT, the program must clear the current in-place status line, print exactly 'Completed N focus sessions\n' where N counts only fully finished focus sessions, and exit with status 0"

**Interfaces**
- Consumes: Timer
- Produces: func handleSIGINTFn

Implement handleSIGINTFn to clear the line using \r\x1b[K and print the completed count.

**Scenarios (write these tests first):**
- S6 `TestInterrupt_S6_FocusInterrupted`: Interrupted session is NOT counted.
- S21 `TestInterrupt_S21_StartInterrupted`: Immediate interrupt counts 0 sessions.
- S22 `TestInterrupt_S22_EndOfSessionInterrupted`: Interrupting 1s before completion does not count the session.

Run: `go test ./... -count=1 -run '^(TestInterrupt_S6_FocusInterrupted|TestInterrupt_S21_StartInterrupted|TestInterrupt_S22_EndOfSessionInterrupted)$'`

### T05: Completion Message Logic

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `completion.go`, `completion_test.go`
**Covers:** S3, S9, S10, S11, S17, S20
**Spec excerpt:** "The writer first receives '\r\x1b[K' to clear the 'Session 3: 05:00' remnant, then 'Completed 2 focus sessions\n'."

**Interfaces**
- Consumes: Timer
- Produces: var runCompletion

Implement runCompletion to use the ANSI clear sequence and format the message with the completed count.

**Scenarios (write these tests first):**
- S3 `TestCompletion_S3_LongBreak`: Long break interrupted prints correct count.
- S9 `TestCompletion_S9_ClearLine`: SIGINT clears the current line before printing completion.
- S10 `TestCompletion_S10_AfterSession4`: Completes 4 sessions correctly.
- S11 `TestCompletion_S11_Start`: Initial interrupt counts 0.
- S17 `TestCompletion_S17_FocusInterrupted`: Interrupted session not counted.
- S20 `TestCompletion_S20_Session5Start`: Counts previous cycle's completed sessions.

Run: `go test ./... -count=1 -run '^(TestCompletion_S3_LongBreak|TestCompletion_S9_ClearLine|TestCompletion_S10_AfterSession4|TestCompletion_S11_Start|TestCompletion_S17_FocusInterrupted|TestCompletion_S20_Session5Start)$'`

### T06: Flag Validation

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `flags.go`, `flags_test.go`
**Covers:** S4, S12, S13, S15, S19
**Spec excerpt:** "The program must validate that each configured duration... does not exceed 24 hours at startup"

**Interfaces**
- Consumes: nothing
- Produces: var validateFlagsFn

Implement validateFlagsFn to check durations against the 24h limit and the 1s minimum, returning exit code 1 on failure.

**Scenarios (write these tests first):**
- S4 `TestFlags_S4_TooLong`: Rejects duration > 24h.
- S12 `TestFlags_S12_ErrorMsg`: Prints descriptive error message.
- S13 `TestFlags_S13_MultipleErrors`: Reports multiple errors.
- S15 `TestFlags_S15_NonPositive`: Rejects zero or negative durations.
- S19 `TestFlags_S19_DuplicateCheck`: Identical error for same violation.

Run: `go test ./... -count=1 -run '^(TestFlags_S4_TooLong|TestFlags_S12_ErrorMsg|TestFlags_S13_MultipleErrors|TestFlags_S15_NonPositive|TestFlags_S19_DuplicateCheck)$'`
