# CLI Configuration & Validation — story plan

> **For agentic workers:** execute wave by wave; qa writes tests, coder implements.

**Goal:** Implement CLI flag parsing, validation, and the main entry point wiring for the Pomodoro timer.
**Architecture:** A contract task defines the `runCLI` signature and `runTimerFn` stub. Two implementation tasks (parsing and validation) depend on this contract. An integration task wires them to the main entry point.
**Tech Stack:** Go, standard library only
**Spec:** docs/pipeline/stories/C0005-cli-configuration-validation.contract.md    **Story Contract:** docs/pipeline/stories/C0005-cli-configuration-validation.contract.json
**Context:** tools
**Lint:** `ai-mode plan lint` exit 0 · **Verify:** `ai-mode plan verify --task all` exit 0

_Rendered by `ai-mode plan render` from the plan JSON next to this file; change the JSON, not this file._

## Global Constraints
- Language: Go (standard library only).
- Platform: macOS Terminal.
- No external dependencies or network/disk access.
- Maximum session length must not exceed 24 hours to prevent resource exhaustion.

## Review Focus
1. Validation order (-focus, then -short, then -long) (S9, S10)
2. Exit status 2 on validation failure (S2, S3, S4, S7, S8)
3. Error message exactness (S3, S4, S7, S8)
4. Default durations when flags are omitted (S11)
5. Flag application to Timer fields (S1, S5, S6, S12)

## Boundaries
| Boundary | Contract artifact | Fixtures | Fake | Provider check | Owner task |
| --- | --- | --- | --- | --- | --- |
| CLI Seam | cli_contract.go | none | none | cli_contract_test.go | T00 |
| Program Entry Point | timer_types.go | none | none | timer_types_test.go | T03 |

## Task graph

```json
{
 "hot_files": [
  "go.mod",
  "README.md"
 ],
 "tasks": [
  {
   "id": "T00",
   "title": "Contract: CLI Seam",
   "kind": "contract",
   "packet": "code",
   "depends_on": [],
   "owns": [
    "cli_contract.go"
   ],
   "produces": [
    "func runCLI(args []string, out io.Writer) int",
    "var runTimerFn func(context.Context, time.Duration, time.Duration, time.Duration) error"
   ],
   "consumes": [],
   "covers": [],
   "test_level": "contract",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T01",
   "title": "Implement CLI Parsing - Primary",
   "kind": "impl",
   "packet": "spec",
   "depends_on": [
    "T00"
   ],
   "owns": [
    "cli_logic.go",
    "cli_logic_test.go"
   ],
   "produces": [
    "Implementation of flag parsing logic"
   ],
   "consumes": [
    "runCLI"
   ],
   "covers": [
    "S2",
    "S3",
    "S4",
    "S5",
    "S6"
   ],
   "test_level": "functional",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T02",
   "title": "Implement CLI Parsing - Secondary",
   "kind": "impl",
   "packet": "spec",
   "depends_on": [
    "T00"
   ],
   "owns": [
    "cli_logic_secondary.go",
    "cli_logic_secondary_test.go"
   ],
   "produces": [
    "Implementation of flag parsing logic"
   ],
   "consumes": [
    "runCLI"
   ],
   "covers": [
    "S7",
    "S8",
    "S9",
    "S10",
    "S11",
    "S12"
   ],
   "test_level": "functional",
   "stack_layer": 1,
   "branch": ""
  },
  {
   "id": "T03",
   "title": "Wire main to runCLI",
   "kind": "integration",
   "packet": "spec",
   "depends_on": [
    "T00"
   ],
   "owns": [
    "timer_types.go",
    "timer_types_test.go"
   ],
   "produces": [
    "func main() calls runCLI"
   ],
   "consumes": [
    "runCLI"
   ],
   "covers": [
    "S1"
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
| S1 | R1 | T03 | `TestMain_S1_Journey` |
| S2 | R1 | T01 | `TestCLI_S2_FocusNegative` |
| S3 | R1 | T01 | `TestCLI_S3_FocusZero` |
| S4 | R1 | T01 | `TestCLI_S4_FocusTooLarge` |
| S5 | R1 | T01 | `TestCLI_S5_FocusValid` |
| S6 | R1 | T01 | `TestCLI_S6_ShortValid` |
| S7 | R1 | T02 | `TestCLI_S7_LongTooLarge` |
| S8 | R2 | T02 | `TestCLI_S8_ShortTooLarge` |
| S9 | R3 | T02 | `TestCLI_S9_ValidationOrder` |
| S10 | R3 | T02 | `TestCLI_S10_ValidationOrderZero` |
| S11 | R4 | T02 | `TestCLI_S11_Defaults` |
| S12 | R4 | T02 | `TestCLI_S12_PartialFlags` |

## Task packets

### T00: Contract: CLI Seam

**Kind / packet / stack layer / test level / depends on:** contract / code / 1 / contract / none
**Owns (exclusive):** `cli_contract.go`
**Covers:** none
**Spec excerpt:** none

**Interfaces**
- Consumes: nothing
- Produces: func runCLI(args []string, out io.Writer) int; var runTimerFn func(context.Context, time.Duration, time.Duration, time.Duration) error

**Phase A: the contract code.**

```go file=cli_contract.go
package main

import "io"

// runCLI parses flags and starts the timer.
// Returns the exit code.
var runCLI = func(args []string, out io.Writer) int {
	return 0
}
```

Run: `go vet ./...`

### T01: Implement CLI Parsing - Primary

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / functional / T00
**Owns (exclusive):** `cli_logic.go`, `cli_logic_test.go`
**Covers:** S2, S3, S4, S5, S6
**Spec excerpt:** "The program must validate all three durations (-focus, -short, -long)" and "defaults apply only to flags that were not supplied"

**Interfaces**
- Consumes: runCLI
- Produces: Implementation of flag parsing logic

Handles the first set of flags and basic parsing.

**Scenarios (write these tests first):**
- S2 `TestCLI_S2_FocusNegative`: Given args '-focus -1', runCLI returns exit status 2
- S3 `TestCLI_S3_FocusZero`: Given args '-focus 0', runCLI returns exit status 2 and message 'Invalid value for -focus: must be between 1 and 1440 minutes.'
- S4 `TestCLI_S4_FocusTooLarge`: Given args '-focus 1441', runCLI returns exit status 2
- S5 `TestCLI_S5_FocusValid`: Given args '-focus 1440', runTimerFn is called with 1440, 5, 15 minutes
- S6 `TestCLI_S6_ShortValid`: Given args '-short 1', runTimerFn is called with 25, 1, 15 minutes

Run: `go test ./... -count=1 -run '^(TestCLI_S2_FocusNegative|TestCLI_S3_FocusZero|TestCLI_S4_FocusTooLarge|TestCLI_S5_FocusValid|TestCLI_S6_ShortValid)$'`

### T02: Implement CLI Parsing - Secondary

**Kind / packet / stack layer / test level / depends on:** impl / spec / 1 / functional / T00
**Owns (exclusive):** `cli_logic_secondary.go`, `cli_logic_secondary_test.go`
**Covers:** S7, S8, S9, S10, S11, S12
**Spec excerpt:** "Validation must check -focus, then -short, then -long in that fixed order" and "defaults apply only to flags that were not supplied"

**Interfaces**
- Consumes: runCLI
- Produces: Implementation of flag parsing logic

Handles the remaining flags, validation order, and defaults.

**Scenarios (write these tests first):**
- S7 `TestCLI_S7_LongTooLarge`: Given args '-long 1s', runCLI returns exit status 2
- S8 `TestCLI_S8_ShortTooLarge`: Given args '-short 1441', runCLI returns exit status 2
- S9 `TestCLI_S9_ValidationOrder`: Given args '-focus 200 -short -1 -long 5', runCLI returns exit status 2 for -focus
- S10 `TestCLI_S10_ValidationOrderZero`: Given args '-focus 0 -short -1 -long 5', runCLI returns exit status 2 for -focus
- S11 `TestCLI_S11_Defaults`: Given no args, runCLI calls runTimerFn with 2int, 5, 15 minutes
- S12 `TestCLI_S12_PartialFlags`: Given args '-focus 50', runCLI calls runTimerFn with 50, 5, 15 minutes

Run: `go test ./... -count=1 -run '^(TestCLI_S7_LongTooLarge|TestCLI_S8_ShortTooLarge|TestCLI_S9_ValidationOrder|TestCLI_S10_ValidationOrderZero|TestCLI_S11_Defaults|TestCLI_S12_PartialFlags)$'`

### T03: Wire main to runCLI

**Kind / packet / stack layer / test level / depends on:** integration / spec / 1 / functional / T00
**Owns (exclusive):** `timer_types.go`, `timer_types_test.go`
**Covers:** S1
**Spec excerpt:** none

**Interfaces**
- Consumes: runCLI
- Produces: func main() calls runCLI

Refactor main in timer_types.go to call runCLI(os.Args[1:], os.Stdout).

**Scenarios (write these tests first):**
- S1 `TestMain_S1_Journey`: Running the program with '-focus 50 -short 10' successfully invokes the timer engine

Run: `go test ./... -count=1 -run '^(TestMain_S1_Journey)$'`
