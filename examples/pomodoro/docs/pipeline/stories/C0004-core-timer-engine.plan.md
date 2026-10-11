# Core Timer Engine — story plan

> **For agentic workers:** execute wave by wave; qa writes tests, coder implements.

**Goal:** Implement a lightweight, terminal-based Pomodoro timer with a 4-session cycle and SIGINT handling.
**Architecture:** A modular approach using package-level function variables for the core loop, output formatting, signal handling, and flag validation, allowing for incremental implementation and testing of each component. The core types are defined in `timer_types.go`.
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
1. The transition logic for the 4-session cycle (S1, S2, S16)
2. The ANSI escape sequence accuracy for in-place updates (S5, S7, S8, S14, S18)
3. The SIGINT exit behavior and correct completion count (S3, S6, S9, S10, S11, S17, S20, S21, S22)
4. Command-line argument validation for the 24-hour limit (S4, S12, S13, S15, S19)

## Boundaries
| Boundary | Contract artifact | Fixtures | Fake | Provider check | Owner task |
| --- | --- | --- | --- | --- | --- |
| time | Clock interface | none | fakeClock in timer_test.go | loop_test.go | T01 |
| terminal output | io.Writer | none | bytes.Buffer in tests | output_test.go | T01 |

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
   "title": "Contract: main.go entry point and seams",
   "kind": "contract",
   "packet": "code",
   "depends_on": [],
   "owns": [
    "main.go"
   ],
   "produces": [
    "main function and function variable seams"
   ],
   "consumes": [],
   "covers": [],
   "test_level": "contract",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T02",
   "title": "Impl: Timer Loop Logic",
   "kind": "impl",
   "packet": "spec",
   "depends_on": [
    "T01"
   ],
   "owns": [
    "loop.go",
    "loop_test.go"
   ],
   "produces": [
    "func runLoopFn"
   ],
   "consumes": [
    "Timer"
   ],
   "covers": [
    "S1",
    "S2",
    "S16",
    "S14"
   ],
   "test_level": "unit",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T03",
   "title": "Impl: Terminal Output Formatting",
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
    "func updateOutputFn"
   ],
   "consumes": [],
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
   "title": "Impl: SIGINT - Interrupted Session",
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
   "title": "Impl: SIGINT - Completion Message",
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
    "func handleSIGINTFn"
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
   "title": "Impl: CLI and Flags",
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
    "func validateFlagsFn"
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
| S1 | R1 | T02 | `TestLoop_S1_ShortBreakAfterFocus` |
| S2 | R1 | T02 | `TestLoop_S2_FullCycle` |
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
| S14 | R2 | T02 | `TestLoop_S14_Boundary` |
| S15 | R2 | T06 | `TestFlags_S15_NonPositive` |
| S16 | R5 | T02 | `TestLoop_S16_CycleRestart` |
| S17 | R6 | T05 | `TestCompletion_S17_FocusInterrupted` |
| S18 | R2 | T03 | `TestOutput_S18_Initial` |
| S19 | R7 | T06 | `TestFlags_S19_DuplicateCheck` |
| S20 | R8 | T05 | `TestCompletion_S20_Session5Start` |
| S21 | R8 | T04 | `TestInterrupt_S21_StartInterrupted` |
| S22 | R8 | T04 | `TestInterrupt_S22_EndOfSessionInterrupted` |

## Task packets

### T01: Contract: main.go entry point and seams

**Kind / packet / stack layer / test level / depends on:** contract / code / 1 / contract / none
**Owns (exclusive):** `main.go`
**Covers:** none
**Spec excerpt:** none

**Interfaces**
- Consumes: nothing
- Produces: main function and function variable seams

**Phase A: the contract code.**

```go file=main.go
package main

import (
	"context"
	"time"
)

// Seams for modular testing
var runLoopFn = func(ctx context.Context, t *Timer) error { return nil }
var updateOutputFn = func(t *Timer, out interface{}) error { return nil }
var handleSIGINTFn = func(ctx context.Context, t *Timer) error { return nil }
var validateFlagsFn = func(durations map[string]time.Duration) error { return nil }

// main is provided for contract verification in timer_types.go.
// We leave it empty here to avoid redeclaration errors if timer_types.go also defines it.
```

Run: `go vet ./...`

### T02: Impl: Timer Loop Logic

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `loop.go`, `loop_test.go`
**Covers:** S1, S2, S16, S14
**Spec excerpt:** "The timer must transition between focus sessions, short breaks, and long breaks according to the four-session cycle logic"

**Interfaces**
- Consumes: Timer
- Produces: func runLoopFn

Implement the loop that calls updateOutputFn and handles duration ticks.

**Scenarios (write these tests first):**
- S1 `TestLoop_S1_ShortBreakAfterFocus`: Transition to ShortBreak after 1 focus session
- S2 `TestLoop_S2_FullCycle`: Transition to LongBreak after 4 focus sessions
- S16 `TestLoop_S16_CycleRestart`: Cycle restarts after LongBreak
- S14 `TestLoop_S14_Boundary`: Transitions exactly at duration limit

Run: `go test ./... -count=1 -run '^(TestLoop_S1_ShortBreakAfterFocus|TestLoop_S2_FullCycle|TestLoop_S16_CycleRestart|TestLoop_S14_Boundary)$'`

### T03: Impl: Terminal Output Formatting

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `output.go`, `output_test.go`
**Covers:** S5, S7, S8, S18
**Spec excerpt:** "The timer must update the in-place status line every second using carriage return and ANSI erase-to-end-of-line"

**Interfaces**
- Consumes: nothing
- Produces: func updateOutputFn

Implement the logic that uses io.Writer to print the current state.

**Scenarios (write these tests first):**
- S5 `TestOutput_S5_Formatting`: Prints 'Session 1: 25:00\r\x1b[K'
- S7 `TestOutput_S7_ShortBreak`: Transition to ShortBreak shows 'Break: 05:00\r\x1b[K'
- S8 `TestOutput_S8_LongBreak`: Long break shows 'Long break: 15:00\r\x1b[K'
- S18 `TestOutput_S18_Initial`: Initial output at t=0 is correct

Run: `go test ./... -count=1 -run '^(TestOutput_S5_Formatting|TestOutput_S7_ShortBreak|TestOutput_S8_LongBreak|TestOutput_S18_Initial)$'`

### T04: Impl: SIGINT - Interrupted Session

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `interrupt.go`, `interrupt_test.go`
**Covers:** S6, S21, S22
**Spec excerpt:** "On SIGINT, the program must clear the current in-place status line, print exactly 'Completed N focus sessions\n'"

**Interfaces**
- Consumes: Timer
- Produces: func handleSIGINTFn

Implement the logic for handling interruption during a session.

**Scenarios (write these tests first):**
- S6 `TestInterrupt_S6_FocusInterrupted`: Focus session interrupted doesn'int count
- S21 `TestInterrupt_S21_StartInterrupted`: Session 1 interrupted at t=1s doesn't count
- S22 `TestInterrupt_S22_EndOfSessionInterrupted`: Session 4 interrupted 1s before end doesn't count

Run: `go test ./... -count=1 -run '^(TestInterrupt_S6_FocusInterrupted|TestInterrupt_S21_StartInterrupted|TestInterrupt_S22_EndOfSessionInterrupted)$'`

### T05: Impl: SIGINT - Completion Message

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `completion.go`, `completion_test.go`
**Covers:** S3, S9, S10, S11, S17, S20
**Spec excerpt:** "On SIGINT, the program must clear the current in-place status line, print exactly 'Completed N focus sessions\n'"

**Interfaces**
- Consumes: Timer
- Produces: func handleSIGINTFn

Implement the logic for printing the final message.

**Scenarios (write these tests first):**
- S3 `TestCompletion_S3_LongBreak`: Prints correct count when LongBreak interrupted
- S9 `TestCompletion_S9_ClearLine`: Clears line before printing message
- S10 `TestCompletion_S10_AfterSession4`: Prints correct count after session 4 completed
- S11 `TestCompletion_S11_Start`: Prints correct count at start
- S17 `TestCompletion_S17_FocusInterrupted`: Prints correct count after focus interrupted
- S20 `TestCompletion_S20_Session5Start`: Prints correct count at session 5 start

Run: `go test ./... -count=1 -run '^(TestCompletion_S3_LongBreak|TestCompletion_S9_ClearLine|TestCompletion_S10_AfterSession4|TestCompletion_S11_Start|TestCompletion_S17_FocusInterrupted|TestCompletion_S20_Session5Start)$'`

### T06: Impl: CLI and Flags

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / unit / T01
**Owns (exclusive):** `flags.go`, `flags_test.go`
**Covers:** S4, S12, S13, S15, S19
**Spec excerpt:** "The program must validate that each configured duration... does not exceed 24 hours at startup"

**Interfaces**
- Consumes: nothing
- Produces: func validateFlagsFn

Implement the flag validation logic.

**Scenarios (write these tests first):**
- S4 `TestFlags_S4_TooLong`: Reject 25h duration
- S12 `TestFlags_S12_ErrorMsg`: Prints error to stderr
- S13 `TestFlags_S13_MultipleErrors`: Prints multiple errors
- S15 `TestFlags_S15_NonPositive`: Handles non-positive durations
- S19 `TestFlags_S19_DuplicateCheck`: Prints error for 2rad 25h duration

Run: `go test ./... -count=1 -run '^(TestFlags_S4_TooLong|TestFlags_S12_ErrorMsg|TestFlags_S13_MultipleErrors|TestFlags_S15_NonPositive|TestFlags_S19_DuplicateCheck)$'`
