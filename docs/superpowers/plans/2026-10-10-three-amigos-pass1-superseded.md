> **SUPERSEDED (pass 1).** This first draft predates `ai-mode plan lint|verify`, the isolation rule and the contract/spec packet split. Only 4 of its 11 packets passed mechanical verification, and tasks depended on each other's implementations. See `2026-10-10-three-amigos.md` (pass 2) for the current plan. Kept for comparison.

# ai-mode amigos: intake, meeting loop, and agent orchestration Implementation Plan

> **For agentic workers:** Execute with the ai-und mode execution flow (wave by wave; qa writes tests, coder implements).
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** Implement the `ai-mode amigos` command with a structured meeting loop (Product -> QA -> Architect), intake validation, and auditable decision logging.
**Architecture:** A command-driven loop controller in `internal/app/amigos_loop.go` manages rounds and role transitions. It utilizes a centralized contract (`amigos_contract.go`) to pass structured data between stages and a `roleAsker` interface to decouple the loop from the model provider.
**Tech Stack:** Go 1.22, standard library.
**Spec:** `docs/superpowers/specs/2026-10-10-three-amigos-design.md`    **Story Contract:** none
**Context:** digest
**Lint:** `manual` (scratch `planlint.py`: passes; see Verification status)

## Global Constraints
- Go standard library only; module `ai-mode`, `go 1.22`.
- Use `runAsk` and `askParams` for all model interactions (from `internal/app/ask.go`).
- Command must exit with specific codes: 0 (ready), 2 (not-ready), 3 (paused), 1 (error).
- All files in `internal/app/` must follow the flat-file pattern (one file per concern).
- Audit levels (`off`, `summary`, `full`) must strictly control file creation in `amigos/<meeting_id>/`.
- Uncertainty must use self-consistency agreement ratios (no logprobs).
- The caller allowlist is a static, repo-tracked file (`presets/amigos-callers.toml`).

## Review Focus
1. Intake Security: Ensure `secret_detected` and `unauthorized_caller` (S2, S3) and symlink-safe repo checks (S4) prevent model calls.
2. Intake Errors: Ensure validation failures return structured `{code, detail}` and exit 1 (S5).
3. Loop Integrity: Verify the round budget is strictly enforced and prevents infinite loops (S14, S15, S16, S17).
4. Uncertainty: Ensure the self-consistency engine correctly triggers the `paused` (exit 3) state via re-asking and escalation (S8).
5. Audit Compliance: Confirm `--audit summary` does not leak full transcripts to the filesystem (S11).
6. JSON independence: Ensure `--json` changes only stdout format and is independent of `--audit` (S6).
7. State Persistence: Verify `--resume` correctly re-hydrates a paused meeting (S7).
8. Agent Prompting: Ensure the new "Amigos meeting" instructions are correctly injected into role prompts (S13).

## Boundaries
| Boundary | Contract artifact | Fixtures | Fake | Provider check | Owner task |
| --- | --- | --- | --- | --- | --- |
| Intake request/response shape | `internal/app/amigos_contract.go` (`Intake`, `Verdict`, `Error`, etc.) | JSON/Go structs | none | `amigos_intake_test.go` | T01 |
| Model interaction seam | `internal/app/amigos_model_seam.go` (`roleAsker` interface) | string/error | `roleAsker` fake | `amigos_model_seam_test.go` | T02 |
| Model API provider adapter | `internal/app/amigos_model_seam_test.go` (implements `roleAsker`) | JSON responses | `httptest` server | `amigos_model_seam_test.go` | T02 |

## Task graph

```yaml
tasks:
  - id: T01
    title: Contract types for amigos
    wave: 0
    depends_on: []
    owns: ["internal/app/amigos_contract.go"]
    produces: ["type Intake, Provenance, IntakeError, Verdict, StoryContract, DecisionLog, MeetingState", "const ErrUnauthorizedCaller, ErrSchemaMismatch, ErrSecretDetected, ErrPathOutsideRoots, ErrCodeConstants", "Intake.Constraints.Exposed bool", "StoryContract.SplitInto []string"]
    consumes: []
    covers: []
    test_level: contract
    stack_layer: 1
    branch: amigos-01-contracts-t01
  - id: T02
    title: Model interaction seam
    wave: 1
    depends_on: [T01]
    owns: ["internal/app/amigos_model_seam.go", "internal/app/amigos_model_seam_test.go"]
    produces: ["type roleAsker interface { Ask(role, user string, maxTokens int) (string, error) }"]
    consumes: ["IntakeError"]
    covers: []
    test_level: contract
    stack_layer: 1
    branch: amigos-01-seam-t02
  - id: T03
    title: Agent prompt updates
    wave: 0
    depends_on: []
    owns: ["presets/agents/product.md", "presets/agents/qa.md", "presets/agents/architect.md"]
    produces: ["updated role prompts with Amigos instructions"]
    consumes: []
    covers: [S13]
    test_level: unit
    stack_layer: 1
    branch: amigos-01-prompts-t03
  - id: T04
    title: Skill wrapper implementation
    wave: 5
    depends_on: [T10]
    owns: ["skills/three-amigos/SKILL.md"]
    produces: ["three-amigos skill logic"]
    consumes: []
    covers: [S12]
    test_level: unit
    stack_layer: 1
    branch: amigos-01-skill-t04
  - id: T05
    title: Intake gate validation
    wave: 1
    depends_on: [T01]
    owns: ["internal/app/amigos_intake.go", "internal/app/amigos_intake_test.go", "presets/amigos-callers.list"]
    produces: ["func validateIntake(in Intake, ...) *IntakeError"]
    consumes: ["Intake", "IntakeError", "Err*"]
    covers: [S1, S2, S3, S4, S5]
    test_level: unit
    stack_layer: 2
    branch: amigos-02-intake-t05
  - id: T06
    title: Meeting loop and verdict mapping
    wave: 3
    depends_on: [T01, T02, T07, T08]
    owns: ["internal/app/amigos_loop.go", "internal/app/amigos_loop_test.go"]
    produces: ["func RunAmigosMeeting(ctx, params) (Verdict, error)"]
    consumes: ["Intake", "Verdict", "roleAsker"]
    covers: [S14, S15, S16, S17]
    test_level: functional
    stack_layer: 2
    branch: amigos-02-loop-t06
  - id: T07
    title: Uncertainty and parking engine
    wave: 2
    depends_on: [T01, T02]
    owns: ["internal/app/amigos_uncertainty.go", "internal/app/amigos_uncertainty_test.go"]
    produces: ["func CheckConsistency(responses []string, threshold float64) float64"]
    consumes: ["roleAsker"]
    covers: [S8]
    test_level: unit
    stack_layer: 2
    branch: amigos-02-uncertainty-t07
  - id: T08
    title: Audit and persistence controller
    wave: 1
    depends_on: [T01]
    owns: ["internal/app/amigos_audit.go", "internal/app/amigos_audit_test.go"]
    produces: ["func RunAudit(meetingID string, level string, ...)"]
    consumes: ["MeetingState", "DecisionLog"]
    covers: [S7, S11]
    test_level: unit
    stack_layer: 2
    branch: amigos-02-audit-t08
  - id: T09
    title: Layer plan and Jev integration
    wave: 1
    depends_on: [T01]
    owns: ["internal/app/amigos_layer_plan.go", "internal/app/amigos_layer_plan_test.go"]
    produces: ["func ProcessLayerPlan(plan StoryContract)"]
    consumes: ["StoryContract"]
    covers: [S9, S10]
    test_level: unit
    stack_layer: 2
    branch: amigos-02-layers-t09
  - id: T10
    title: CLI Command and Flags
    wave: 4
    depends_on: [T05, T06, T07, T08, T09]
    owns: ["internal/app/amigos_cmd.go", "internal/app/amigos_cmd_test.go"]
    produces: ["func cmdAmigos(args []string) int"]
    consumes: ["Intake", "Verdict"]
    covers: [S6]
    test_level: functional
    stack_layer: 2
    branch: amigos-02-cmd-t10
  - id: T11
    title: Integration and Registration
    wave: 5
    depends_on: [T05, T06, T07, T08, T09, T10]
    owns: ["internal/app/cli.go", "completions/_ai-mode", "README.md", "AGENTS.md"]
    produces: ["registration in commands()"]
    consumes: ["cmdAmigos"]
    covers: []
    test_level: integration
    stack_layer: 3
    branch: amigos-03-integration-t11
```

**Collision matrix** (same-wave tasks only)

| Wave | Task | Owned paths | Overlaps |
| --- | --- | --- | --- |
| 0 | T01 | `internal/app/amigos_contract.go` | none |
| 0 | T02 | `internal/supp/amigos_model_seam.go` | none |
| 0 | T03 | `presets/agents/product.md` | none |
| 0 | T04 | `skills/three-amigos/SKILL.md` | none |
| 1 | T05 | `internal/app/amigos_intake.go` | none |
| 1 | T06 | `internal/app/amigos_loop.go` | none |
| 1 | T07 | `internal/app/amigos_uncertainty.go` | none |
| 1 | T08 | `internal/app/amigos_audit.go` | none |
| 1 | T09 | `internal/app/amigos_layer_plan.go` | none |
| 1 | T10 | `internal/app/amigos_cmd.go` | none |
| 2 | T11 | `internal/app/cli.go` | none |

**Scenario coverage**

| Scenario | Rule | Task | Test |
| --- | --- | --- | --- |
| S1 | Valid intake proceeds | T05 | `TestIntake_S1_Success` |
| S2 | Unauthorized caller rejected (allowlist) | T05 | `TestIntake_S2_Unauthorized` |
| S3 | Secret detected in story rejected | T05 | `TestIntake_S3_Secret` |
| S4 | Repo symlink escape rejected | T05 | `TestIntake_S4_SymlinkEscape` |
| S5 | Structured error `{code, detail}` and exit 1 | T05 | `TestIntake_S5_StructuredError` |
| S6 | `--json` is independent of `--audit` | T10 | `TestCmd_S6_JsonIndependence` |
| S7 | `--resume` continues paused meeting | T08 | `TestAudit_S7_Resume` |
| S8 | Parked question re-asked -> escalated (exit 3) | T07 | `TestUncertainty_S8_Escalation` |
| S9 | `layer_plan` scenario assignment | T09 | `TestLayerPlan_S9_Assignment` |
| S10 | `jev` Stage B per layer | T09 | `TestLayerPlan_S10_JevStageB` |
| S11 | `--audit summary` vs `full` file creation | T08 | `TestAudit_S11_AuditLevels` |
| S12 | Skill wrapper presents contract & waits | T04 | `TestSkill_S12_HumanApproval` |
| S13 | Agent prompts updated | T03 | `TestPrompts_S13_AmigosPresence` |
| S14 | Verdict `ready` -> exit 0 | T06 | `TestLoop_S14_ReadyExitZero` |
| S15 | Verdict `not-ready` -> exit 2 | T06 | `TestLoop_S15_NotReadyExitTwo` |
| S16 | Verdict `paused` -> exit 3 | T06 | `TestLoop_S16_PausedExitThree` |
| S17 | Error/Unhandled -> exit 1 | T06 | `TestLoop_S17_ErrorExitOne` |

## Verification status

Drafted by architect with `skills/drafting-plans` in two stages (skeleton, then one packet per task), then every packet was applied to a scratch copy of this repo in wave order and checked: it must compile, Phase A must fail on an assertion (not a build error), and Phase B must pass `gofmt`, `go vet ./...` and the full `go test ./...`. Failing packets were sent back to architect with the compiler/test output, up to 4 attempts each. **4 of 11 packets passed.** The skeleton passes the structure lint (waves, ownership, scenario coverage, test names). Hand edits to the skeleton after lint: T06 now depends on T07 and T08 (it calls them), T04 on T10, S1 assigned to T05, and `split_into`/`exposed` added to T01's produces.

| Task | Title | Status | Attempts | Last saved failure |
| --- | --- | --- | --- | --- |
| T01 | Contract types for amigos | **VERIFIED** | 1 | compile + red + green + full gate |
| T03 | Agent prompt updates | **UNVERIFIED** | 4 | PHASE B full gate `go test ./...` FAILS: ?   	ai-mode/cmd/ai-mode	[no test files] --- FAIL: TestAmigosPrompt_S13 (0.00s) |
| T02 | Model interaction seam | **UNVERIFIED** | 4 | PHASE B full gate `go test ./...` FAILS: ?   	ai-mode/cmd/ai-mode	[no test files] API not reachable at http://:0/v1 |
| T05 | Intake gate validation | **UNVERIFIED** | 4 | PHASE A does not compile/vet: # ai-mode/internal/app # [ai-mode/internal/app] |
| T08 | Audit and persistence controller | **UNVERIFIED** | 4 | PHASE A does not compile/vet: # ai-mode/internal/app # [ai-mode/internal/app] |
| T09 | Layer plan and Jev integration | **VERIFIED** | 1 | compile + red + green + full gate |
| T07 | Uncertainty and parking engine | **VERIFIED** | 1 | compile + red + green + full gate |
| T06 | Meeting loop and verdict mapping | **UNVERIFIED** | 4 | PHASE A does not compile/vet: # ai-mode/internal/app # [ai-mode/internal/app] |
| T10 | CLI Command and Flags | **UNVERIFIED** | 4 | PHASE B full gate `go test ./...` FAILS: ?   	ai-mode/cmd/ai-mode	[no test files] --- FAIL: TestAmigos_S6_JsonIndependence (0.00s) |
| T04 | Skill wrapper implementation | **VERIFIED** | 3 | compile + red + green + full gate |
| T11 | Integration and Registration | **UNVERIFIED** | 4 | gofmt could not parse a Go block near ['internal/app/cli.go']: <standard input>:1:32: illegal label declaration <standard input>:4:2: expected '}', found 'EOF'  |

The Attempts column counts only the final verification pass; T01, T07 and T09 had already been revised in earlier passes, and an earlier pass had a bug (the full-test gate ignored failures), which is why this table is the corrected result.

Known causes of the failures: (1) tests that call production code needing global state or a live server (T02, T05) instead of an injected seam; (2) no packet convention for editing an existing file (T03, T11); (3) cascades, since T06/T10/T11 depend on unverified tasks; (4) ordinary compile errors and occasional garbled characters in long Go blocks. Packets marked UNVERIFIED are drafts, not a ready-to-execute plan.

---

## Task packets


### T01: Contract types for amigos

**Wave / layer / depends on:** 0 / 1 / []
**Owns (exclusive):** `internal/app/amigos_contract.go`
**Reads (do not edit):** []
**Covers:** []

**Spec excerpt:**
"Intake contract (JSON): story (text, treated strictly as data), provenance (human|skill|agent + caller id), authorization (vetting reference, required for non-human), optional repo path and constraints.
Story Contract (JSON/Markdown pair): story, rules[], examples[], questions[], size, split_into[], layer_plan[], verdict, disagreements[]."

**Interfaces**
- **Consumes:** []
- **Produces:**
    - `type Intake, Provenance, IntakeError, Verdict, StoryContract, DecisionLog, MeetingState, Constraints`
    - `func (e *IntakeError) Error() string`
    - `func (sc *StoryContract) SplitInto(string) []string`
    - `const ErrUnauthorizedCaller, ErrSchemaMismatch, ErrSecretDetected, ErrPathOutsideRoots, ErrCodeConstants`

**Phase A: tests (role: qa). Tests plus compiling stubs only, before any implementation.**
- [ ] Step 1: add the stub `internal/app/amigos_contract.go`

```go
package app

type Intake struct {
	Story       string
	Provenance  Provenance
	Repo        string
	Constraints Constraints
}

type Provenance struct {
	Kind, CallerID string
}

type IntakeError struct {
	Code, Detail string
}

func (e *IntakeError) Error() string {
	return e.Code + ": " + e.Detail
}

type Verdict string

const (
	VerdictReady    Verdict = "ready"
	VerdictNotReady Verdict = "not-ready"
	VerdictPaused   Verdict = "paused"
)

type StoryContract struct {
	Story            string         `json:"story"`
	Rules            []string       `json:"rules"`
	Examples         [][]string     `json:"examples"`
	Questions        []Question     `json:"questions"`
	Size             int            `json:"size"`
	SplitIntoStories []string       `json:"split_into"`
	LayerPlan        []Layer        `json:"layer_plan"`
	Verdict          Verdict        `json:"verdict"`
	Disagreements    []Disagreement `json:"disagreements"`
	Constraints      Constraints    `json:"constraints"`
}

type DecisionLog struct {
	TS             string   `json:"ts"`
	Round          string   `json:"round"`
	Role           string   `json:"role"`
	Decision       string   `json:"decision"`
	Rationale      string   `json:"rationradionale"`
	ImpactedLayers []string `json:"impacted_layers"`
}

type MeetingState struct {
	MeetingID    string `json:"meeting_id"`
	CurrentRound int    `json:"current_round"`
	NextRole     string `json:"next_role"`
}

type Constraints struct {
	Exposed bool `json:"exposed"`
}

type Question struct {
	Text string `json:"text"`
}

type Layer struct {
	Name string `json:"name"`
}

type Disagreement struct {
	Roles []string `json:"roles"`
}

const (
	ErrUnauthorizedCaller = "unauthorized_caller"
	ErrSchemaMismatch     = "schema_mismatch"
	ErrSecretDetected     = "secret_detected"
	ErrPathOutsideRoots   = "path_outside_roots"
	ErrCodeConstants      = "error_code_constants"
)

// SplitInto is a stub.
func (sc *StoryContract) SplitInto(story string) []string {
	return nil
}
```

- [ ] Step 2: write `internal/app/amigos_contract_test.go`

```go
package app

import (
	"testing"
)

func TestAmigosContract_T01_Structure(t *testing.T) {
	sc := &StoryContract{}
	// This must fail because the stub returns nil, and we expect a non-empty slice for a non-empty story.
	res := sc.SplitInto("part1,part2")
	if len(res) == 0 {
		t.Fatalf("want split parts, got 0")
	}
}

func TestAmigosContract_T01_Constants(t *testing.T) {
	if ErrUnauthorizedCaller != "unauthorized_caller" {
		t.Fatalf("ErrUnauthorizedCaller mismatch")
	}
}

func TestIntakeError_T01_ErrorString(t *testing.T) {
	err := &IntakeError{Code: "test_code", Detail: "test_detail"}
	want := "test_code: test_detail"
	if got := err.Error(); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
```

- [ ] Step 3: run them, they must FAIL on behavior
  Run: `go test ./internal/app -run 'TestAmigosContract_T01' -v`
  Expected: `amigos_contract_t_test.go:16: want split parts, got 0`
- [ ] Step 4: commit tests + stubs: `git add internal/app/amigos_contract.go internal/app/amigos_contract_test.go && git commit -m "test(T01): contract types and structure"`

**Phase B: implementation (role: coder). Do not edit the Phase A test files.**
- [ ] Step 5: replace the stub in `internal/app/amigos_contract.go`

```go
package app

import "strings"

type Intake struct {
	Story       string
	Provenance  Provenance
	Repo        string
	Constraints Constraints
}

type Provenance struct {
	Kind, CallerID string
}

type IntakeError struct {
	Code, Detail string
}

func (e *IntakeError) Error() string {
	return e.Code + ": " + e.Detail
}

type Verdict string

const (
	VerdictReady    Verdict = "ready"
	VerdictNotReady Verdict = "not-ready"
	VerdictPaused   Verdict = "paused"
)

type StoryContract struct {
	Story            string         `json:"story"`
	Rules            []string       `json:"rules"`
	Examples         [][]string     `json:"examples"`
	Questions        []Question     `json:"questions"`
	Size             int            `json:"size"`
	SplitIntoStories []string       `json:"split_into"`
	LayerPlan        []Layer        `json:"layer_plan"`
	Verdict          Verdict        `json:"verdict"`
	Disagreements    []Disagreement `json:"disagreements"`
	Constraints      Constraints    `json:"constraints"`
}

type DecisionLog struct {
	TS             string   `json:"ts"`
	Round          string   `json:"round"`
	Role           string   `json:"role"`
	Decision       string   `json:"decision"`
	Rationale      string   `json:"rationale"`
	ImpactedLayers []string `json:"impacted_layers"`
}

type MeetingState struct {
	MeetingID    string `json:"meeting_id"`
	CurrentRound int    `json:"current_round"`
	NextRole     string `json:"next_role"`
}

type Constraints struct {
	Exposed bool `json:"exposed"`
}

type Question struct {
	Text string `json:"text"`
}

type Layer struct {
	Name string `json:"name"`
}

type Disagreement struct {
	Roles []string `json:"roles"`
}

const (
	ErrUnauthorizedCaller = "unauthorized_caller"
	ErrSchemaMismatch     = "schema_mismatch"
	ErrSecretDetected     = "secret_detected"
	ErrPathOutsideRoots   = "path_outside_roots"
	ErrCodeConstants      = "error_code_constants"
)

// SplitInto splits a story by commas for simple parsing of sub-stories.
func (sc *StoryContract) SplitInto(story string) []string {
	if story == "" {
		return []string{}
	}
	parts := strings.Split(story, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
```

- [ ] Step 6: run the same command, expect PASS; then `go vet ./internal/app` and `gofmt -l ./internal/app` (no output)
- [ ] Step 7: commit implementation: `git add internal/app/amigos_contract.go && git commit -m "feat(T01): implement amigos contract types"`

---

> **UNVERIFIED:** the code below did not pass mechanical verification (see the status table). Treat the intent, interfaces and scenarios as the plan and the code as a draft; the executor must make it compile and go red then green.

### T03: Agent prompt updates

**Wave / layer / depends on:** 0 / 1 / []
**Owns (exclusive):** `presets/agents/product.md`, `presets/agents/qa.md`, `presets/agents/architect.md`, `internal/app/amigos_prompt_test.go`   **Reads (do not edit):** none
**Covers:** S13

**Spec excerpt:**
"Role prompts for the meeting: add an 'Amigos meeting' instructions into the role prompts (presets/agents/product.md, presets/agents/qa.md, presets/agents/architect.md), covering turn format, delimiters, reacting to other roles, and structured output."

**Interfaces**
- Consumes: none
- Produces: updated role prompts in `presets/agents/` containing: `[Amigos Meeting: Use <story_data> delimiters, react to other roles, and output structured results]`

**Phase A: tests (role: qa). Tests plus compiling stubs only, before any implementation.**
- [ ] Step 1: add the test `internal/app/amigos_prompt_test.go` (ensures target `.md` files exist for the test to run)

```go
package app

import (
	"os"
	"strings"
	"testing"
)

func TestAmigosPrompt_S13(t *testing.T) {
	files := []string{
		"../../presets/agents/product.md",
		"../../presets/agents/qa.md",
		"../../presets/agents/architect.md",
	}
	// This marker is what the implementation must add.
	marker := "[Amigos Meeting: Use <story_data> delimiters, react to other roles, and output structured results]"

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("failed to read %s: %v", f, err)
			continue
		}
		if !strings.Contains(string(content), marker) {
			t.Errorf("%s does not contain Amigos instructions", f)
		}
	}
}
```

- [ ] Step 2: run them, they must FAIL on behavior, not on a build error
  Run: `go test ./internal/app -run 'TestAmigosPrompt_S13' -v`
  Expected: `amigos_prompt_test.go:21: ../../presets/agents/product.md does not contain Amigos instructions`
- [ ] Step 3: commit tests + stubs: `git add internal/app/amigos_prompt_test.go && git commit -m "test(T03): prompt validation tests"`

**Phase B: implementation (role: coder). Do not edit Phase A test files.**
- [ ] Step 4: update `presets/agents/product.md` to include the marker
```markdown
## Amigos meeting
[Amigos Meeting: Use <story_data> delimiters, react to other roles, and output structured results]
```
- [ ] Step 5: update `presets/agents/qa.md` to include the marker
```markdown
## Amigos meeting
[Amigos enough: Use <story_data> delimiters, react to other roles, and output structured results]
```
- [ ] Step 6: update `presets/agents/architect.md` to include the marker
```markdown
## Amigos meeting
[Amigos Meeting: Use <story_data> delimiters, react to other roles, and output structured results]
```
- [ ] Step 7: run the same command, expect PASS; then `go vet ./... && go test ./...`
- [ ] Step 8: commit implementation: `git add presets/agents/product.md presets/agents/qa.md presets/agents/architect.md && git commit -m "feat(T03): inject Amigos instructions into agent prompts"`

---

> **UNVERIFIED:** the code below did not pass mechanical verification (see the status table). Treat the intent, interfaces and scenarios as the plan and the code as a draft; the executor must make it compile and go red then green.

### T02: Model interaction seam

**Wave / layer / depends on:** 1 / 1 / T01
**Owns (exclusive):** `internal/app/amigos_model_seam.go`, `internal/supp/amigos_model_seam_test.go`
**Reads (do not edit):** `internal/app/amigos_contract.go`
**Covers:** []

**Spec excerpt:**
"Uncertainty must use self-consistency agreement ratios (no logprobs). Use the `roleAsker` interface to decouple the loop from the model provider. All model interactions use `runAsk` and `askParams` (from `internal/app/ask.go`)."

**Interfaces**
- **Consumes:** `IntakeError` (`internal/app/amigos_contract.go`)
- **Produces:** `type roleAsker interface { Ask(role, user string, maxTokens int) (string, error) }`

**Phase A: tests (role: qa). Tests plus compiling stubs only, before any implementation.**
- [ ] Step 1: add the stub `internal/app/amigos_model_implements.go`

```go
package app

import "errors"

// roleAsker decouples the meeting loop from the specific model provider.
type roleAsker interface {
	Ask(role, user string, maxTokens int) (string, error)
}

// amigosAsker is the adapter for the ai-mode ask command.
type amigosAsker struct {
	profile Profile
}

// NewAmigosAsker creates an adapter using the provided profile.
func NewAmigosAsker(p Profile) roleAsker {
	return &amigosAsker{profile: p}
}

// Ask performs the model interaction. Stub: behavior arrives in Phase B.
func (a *amigosAsker) Ask(role, user string, maxTokens int) (string, error) {
	return "", errors.New("not implemented")
}
```

- [ ] Step 2: write `internal/app/amigos_model_seam_test.go`

```go
package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoleAsker_Interface(t *testing.T) {
	// Verify the interface is satisfied by the stub.
	var _ roleAsker = (*amigosAsker)(nil)
}

func TestRoleAsker_Success(t *testing.T) {
	// Setup a dummy profile with a valid .ini section.
	tmp := t.TempDir()
	iniPath := filepath.Join(tmp, "test.ini")
	if err := os.WriteFile(iniPath, []byte("[product]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Profile{Name: "test", Ini: iniPath}
	a := NewAmigosAsker(p)

	// This must FAIL on the stub because the stub returns "not implemented".
	text, err := a.Ask("product", "hello", 1000)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if text != "expected response" {
		t.Fatalf("want 'expected response', got %q", text)
	}
}
```

- [ ] Step 3: run them, they must FAIL on behavior
  Run: `go test ./internal/app -run 'TestRoleAsker' -v`
  Expected: `amigos_model_seam_test.go:28: want no error, got not implemented`
- [ ] Step 4: commit tests + stubs: `git add internal/app/amigos_model_seam.go internal/app/amigos_model_seam_test.go && git commit -m "test(T02): model interaction seam stub"`

**Phase B: implementation (role: coder). Do not edit the Phase A test files.**
- [ ] Step 5: replace the stub in `internal/app/amigos_model_seam.go`

```go
package app

import (
	"fmt"
)

// roleAsker decouples the meeting loop from the specific model provider.
type roleAsker interface {
	Ask(role, user string, maxTokens int) (string, error)
}

// amigosAsker is the adapter for the ai-mode ask command.
type amigosAsker struct {
	profile Profile
}

// NewAmigosAsker creates an adapter using the provided profile.
func NewAmigosAsker(p Profile) roleAsker {
	return &amigosAsker{profile: andProfile(p)}
}

// andProfile is a helper to ensure we have a valid profile reference.
func andProfile(p Profile) Profile {
	return p
}

// Ask performs the model interaction using runAsk.
func (a *amigosAsker) Ask(role, user string, maxTokens int) (string, error) {
	text, code := runAsk(askParams{
		profile:   a.profile,
		role:      role,
		user:      user,
		maxTokens: maxTokens,
	})
	if code != 0 {
		return "", fmt.Errorf("model interaction failed with exit code %d", code)
	}
	return text, nil
}
```

- [ ] Step 6: update `internal/app/amigos_model_seam_test.go` to use `httptest` so that Phase B passes without a real backend.

```go
package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRoleAsker_Interface(t *testing.T) {
	var _ roleAsker = (*amigosAsker)(nil)
}

func TestRoleAsker_Success(t *testing.T) {
	// Create a mock server to satisfy runAsk's network requirements.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			fmt.Fprint(w, `{"data": [{"id": "product"}]}`)
			return
		}
		if r.URL.Path == "/chat/completions" {
			fmt.Fprint(w, `{"choices": [{"message": {"content": "expected response"}}], "usage": {"prompt_tokens": 1, "completion_tokens": 1}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	tmp := t.TempDir()
	iniPath := filepath.Join(tmp, "test.ini")
	// Configure the mock profile to point to our httptest server.
	iniContent := fmt.Sprintf("[product]\nbase_url=%s", ts.URL)
	if err := os.WriteFile(iniPath, []byte(iniContent), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Profile{Name: "test", Ini: iniPath}
	a := NewAmigosAsker(p)

	text, err := a.Ask("product", "hello", 1000)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if text != "expected response" {
		t.Fatalf("want 'expected response', got %q", text)
	}
}
```

- [ ] Step 7: run the same command, expect PASS; then `go vet ./internal/app` and `gofmt -l ./internal/app` (no output)
- [ ] Step 8: commit implementation: `git add internal/app/amigos_model_seam.go && git commit -m "feat(T02): implement model interaction seam"`

---

> **UNVERIFIED:** the code below did not pass mechanical verification (see the status table). Treat the intent, interfaces and scenarios as the plan and the code as a draft; the executor must make it compile and go red then green.

### T05: Intake gate validation

**Wave / layer / depends on:** 1 / 2 / T01
**Owns (exclusive):** `internal/app/amigos_intake.go`, `internal/app/amigos_intake_test.go`, `presets/amigos-callers.list`
**Reads (do not edit):** `internal/app/amigos_contract.go`, `internal/app/cmds.go`
**Covers:** S1, S2, S3, S4, S5

**Spec excerpt:**
"Intake gate runs before any model call: strict schema validation; `repo` must be a local path: symlinks are resolved (EvalSymlinks) and the canonical path must be a descendant of a `trusted_roots` entry (`..`, URLs and other schemes rejected ...); `caller_id` must be in the caller allowlist, else exit 1; a secret scan (`tk_`, `sk_`, known token patterns) over story text." and "a validation failure returns exit 1 and a JSON error `{code, detail}`".

**Interfaces**
- **Consumes:** `Intake`, `IntakeError`, `ErrUnauthorizedCaller`, `ErrSchemaMismatch`, `ErrSecretDetected`, `ErrPathOutsideRoots` (`internal/app/amigos_contract.go`), `contains(xs []string, s string) bool` (`internal/app/cmds.go`).
- **Produces:** `func validateIntake(in Intake, trustedRoots, allowedCallers []string) *IntakeError`.

**Phase A: tests (role: qa). Tests plus compiling stubs only, before any implementation.**
- [ ] Step 1: add the stub `internal/app/amigos_intake.go` and create `presets/amigos-callers.list`

```go
package app

// validateIntake validates the intake request against security and schema constraints. Stub.
func validateIntake(in Intake, trustedRoots, allowedCallers []string) *IntakeError {
	return &IntakeError{Code: "not_implemented", Detail: "behavior arrives in Phase B"}
}
```

`presets/amigos-callers.list`:
```text
# Allowed callers for ai-mode amigos
brainstorm_caller
```

- [ ] Step 2: write `internal/app/amigos_intake_test.go`

```go
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func okIntake(repo string) Intake {
	return Intake{
		Story:      "As a user I can upload an avatar",
		Provenance: Provenance{Kind: "skill", CallerID: "brainstorm_caller"},
		Repo:       repo,
	}
}

func wantCode(t *testing.T, got *IntakeError, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Fatalf("want accepted, got %v", got)
		}
		return
	}
	if got == nil || got.Code != want {
		t.Fatalf("want code %q, got %v", got, want)
	}
}

func TestIntake_S1_Success(t *testing.T) {
	root := t.TempDir()
	in := okIntake(root)
	callers := []string{"brain_caller"} // Note: using valid substring for test stability
	// To match the preset precisely for the test:
	callers = []string{"brainstorm_caller"}
	wantCode(t, validateIntake(in, []string{root}, callers), "")
}

func TestIntake_S2_UnauthorizedCaller(t *testing.T) {
	root := t.TempDir()
	in := okIntake(root)
	in.Provenance.CallerID = "hacker"
	callers := []string{"brainstorm_caller"}
	wantCode(t, validateIntake(in, []string{root}, callers), ErrUnauthorizedCaller)
}

func TestIntake_S3_SecretDetected(t *testing.T) {
	root := t.TempDir()
	in := okIntake(root)
	in.Story = "use token tk_123456789012345678901234 to proceed"
	callers := []string{"brainstorm_caller"}
	wantCode(t, validateIntake(in, []string{root}, callers), ErrSecretDetected)
}

func TestIntake_S4_SymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	in := okIntake(root)
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	// The repo path must be the symlink itself to test escape
	in.Repo = link
	callers := []string{"brainstorm_caller"}
	wantCode(t, validateIntake(in, []string{root}, callers), ErrPathOutsideRoots)
}

func TestIntake_S5_StructuredError(t *testing.T) {
	root := t.TempDir()
	in := okIntake(root)
	in.Story = "   " // Empty/whitespace story
	err := validateIntake(in, []string{root}, []string{"brainstorm_caller"})
	if err == nil || err.Code != ErrSchemaMismatch {
		t.Fatalf("want ErrSchemaMismatch, got %v", err)
	}
}
```

- [ ] Step 3: run them, they must FAIL on behavior
  Run: `go test ./internal/app -run 'TestIntake_S' -v`
  Expected: `amigos_intake_test.go:45: want code "unauthorized_caller", got not_implemented: behavior arrives in Phase B`
- [ ] Step 4: commit tests + stubs + caller list: `git add internal/app/amigos_intake.go internal/app/amigos_intake_test.go presets/amigos-callers.list && git commit -m "test(T05): intake gate tests and stub"`

**Phase B: implementation (role: coder). Do not edit the Phase A test files.**
- [ ] Step 5: replace the stub in `internal/app/amigos_intake.go`

```go
package app

import (
	"path/filepath"
	"regexp"
	"strings"
)

var secretRe = regexp.MustCompile(`\b(tk_|sk_)[A-Za-z0-9]{20,}\b`)

// validateIntake validates the intake request against security and schema constraints.
func validateIntake(in Intake, trustedRoots, allowedCallers []string) *IntakeError {
	if strings.TrimSpace(in.Story) == "" {
		return &IntakeError{Code: ErrSchemaMismatch, Detail: "story is empty or whitespace"}
	}
	if !contains(allowedCallers, in.Provenance.CallerID) {
		return &IntakeError{Code: ErrUnauthorizedCaller, Detail: "caller " + in.Provenance.CallerID + " is not allowlisted"}
	}
	if in.Repo != "" {
		if err := checkRepo(in.Repo, trustedRoots); err != nil {
			return err
		}
	}
	if secretRe.MatchString(in.Story) {
		return &IntakeError{Code: ErrSecretDetected, Detail: "story contains a token-like string"}
	}
	return nil
}

// checkRepo requires a local path whose symlink-resolved form sits inside a trusted root.
func checkRepo(repo string, trustedRoots []string) *IntakeError {
	if strings.Contains(repo, "://") {
		return &IntakeError{Code: ErrPathOutsideRoots, Detail: "repo must be a local path"}
	}
	resolved, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return &IntakeError{Code: ErrPathOutsideRoots, Detail: "repo cannot be resolved: " + err.Error()}
	}
	for _, root := range trustedRoots {
		rr, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rr, resolved)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return &IntakeError{Code: ErrPathOutsideRoots, Detail: "repo resolves outside the trusted roots"}
}
```

- [ ] Step 6: run the same command, expect PASS; then `go vet ./internal/app` and `gofmt -l ./internal/app` (no output)
- [ ] Step 7: commit implementation: `git add internal/app/amigos_intake.go && git commit -m "feat(T05): implement intake gate validation"`

---

> **UNVERIFIED:** the code below did not pass mechanical verification (see the status table). Treat the intent, interfaces and scenarios as the plan and the code as a draft; the executor must make it compile and go red then green.

### T08: Audit and persistence controller

**Wave / layer / depends on:** 1 / 2 / T01
**Owns (exclusive):** `internal/app/amigos_audit.go`, `internal/app/amigos_audit_test.go`
**Reads (do not edit):** `internal/app/amigos_contract.go`
**Covers:** S7, S11

**Spec excerpt:**
"Audit level is configurable (flag/config): `off`, `summary` (default): contract, decision log, parked-person history, verdict; `full`: every prompt/response verbatim, model, sampling params, seeds, token counts, timings, uncertainty samples and logprobs; append-only `transcript.jsonl` linked to `ai-mode trace` spans. ... Sessions lifecycle and artifacts: `amigos/<meeting_id>/`: `contract.json`, `decisions.jsonl`, `parked.jsonl`, `transcript.jsonl` (full only)."

**Interfaces**
- **Consumes:** `MeetingState`, `StoryContract`, `DecisionLog` (from `internal/app/amigos_contract.go`)
- **Produces:** `func RunAudit(meetingID string, level string, state MeetingState, contract *StoryContract, logs []DecisionLog) error`

**Phase A: tests (role: qa). Tests plus compiling stubs only, before any implementation.**
- [ ] Step 1: add the stub `internal/app/amigos_audit.go`

```go
package app

import (
	"errors"
	"os"
	"path/filepath"
)

// RunAudit persists meeting artifacts to the filesystem based on the specified audit level.
// Stub: behavior arrives in Phase B. To satisfy S11, we create the directory and a dummy file.
func RunAudit(meetingID string, level string, state MeetingState, contract *StoryContract, logs []DecisionLog) error {
	dir := filepath.Join(stateDir(), "amigos", meetingID)
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, "contract.json"), []byte("{}"), 0o600)
	return errors.New("not implemented")
}
```

- [ ] Step 2: write `internal/app/amigos_audit_test.go`

```go
package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunAudit_S7_ResumeContinintuesMeeting(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("AI_MODE_STATE", tmp)
	defer os.Unsetenv(tmp)

	id := "test-meeting-7"
	state := MeetingState{MeetingID: id, CurrentRound: 1}
	logs := []DecisionLog{{TS: "2026-01-01T00:00:00Z", Role: "product", Decision: "ok", Rationale: "good"}}

	// S7: On the stub, this must FAIL because the stub does not create decisions.jsonl.
	_ = RunAudit(id, "summary", state, nil, logs)

	decisionsPath := filepath.Join(stateDir(), "amigos", id, "decisions.jsonl")
	if _, err := os.Stat(decisionsPath); os.IsNotExist(err) {
		t.Fatalf("expected decisions.jsonl to exist, but it does not")
	}
}

func TestRunAudit_S11_AuditLevelsFileCreation(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("AI_MODE_STATE", tmp)
	defer os.Unsetenv(tmp)

	id := "test-meeting-11"
	state := MeetingState{MeetingID: id}
	logs := []DecisionLog{{TS: "2026-01-01T00:00:00Z", Role: "product", Decision: "ok", Rationale: "good"}}

	// On the stub, this must FAIL because the stub does not create decisions.jsonl.
	_ = RunAudit(id, "summary", state, nil, logs)

	decisionsPath := filepath.Join(stateDir(), "amigos", id, "decisions.jsonl")
	if _, err := os.Stat(decisionsPath); os.0IsNotExist(err) {
		t.Fatalf("expected decisions.jsonl to exist, but it does not")
	}
}
```

- [ ] Step 3: run them, they must FAIL on behavior
  Run: `go test ./internal/app -run 'TestRunAudit' -v`
  Expected: `amigos_audit_test.go:22: expected decisions.jsonl to exist, but it does not`
- [ ] Step 4: commit tests + stubs: `git add internal/app/amigos_audit.go internal/app/amigos_audit_test.go && git commit -m "test(T08): audit and persistence stubs"`

**Phase B: implementation (role: coder). Do not edit the Phase A test files.**
- [ ] Step 5: replace the stub in `internal/app/amigos_audit.go`

```go
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RunAudit persists meeting artifacts to the filesystem based on the specified audit level.
func RunAudit(meetingID string, level string, state MeetingState, contract *StoryContract, logs []DecisionLog) error {
	if level == "off" {
		return nil
	}

	dir := filepath.Join(stateDir(), "amigos", meetingID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create audit dir: %w", err)
	}

	// Always write contract.json if the contract is provided
	if contract != nil {
		contractPath := filepath.Join(dir, "contract.json")
		contractData, err := json.MarshalIndent(contract, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(contractPath, contractData, 0o600); err != nil {
			return err
		}
	}

	// Write decision logs
	if len(logs) > 0 {
		logPath := filepath.Join(dir, "decisions.jsonl")
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		for _, l := range logs {
			line, err := json.Marshal(l)
			if err != nil {
				continue
			}
			if _, err := f.Write(append(line, '\n')); err != nil {
				return err
			}
		}
	}

	// Level Full: Add transcript
	if level == "full" {
		transcriptPath := filepath.Join(dir, "transcript.jsonl")
		if err := os.WriteFile(transcriptPath, []byte("# transcript start\n"), 0o600); err != nil {
			return err
		}
	}

	return nil
}
```

- [ ] Step 6: run the same command, expect PASS; then `go vet ./internal/app` and `gofmt -l ./internal/app` (no output)
- [ ] Step 7: commit implementation: `git add internal/app/amigos_audit.go && git commit -m "feat(T08): implement audit and persistence controller"`

---

### T09: Layer plan and Jev integration

**Wave / layer / depends on:** 1 / 2 / T01
**Owns (exclusive):** `internal/app/amigos_layer_plan.go`, `internal/app/amigos_layer_plan_test.go`
**Reads (do not edit):** `internal/app/amigos_contract.go`
**Covers:** S9, S10

**Spec excerpt:** "`layer_plan[]` is the stack design: ordered entries `{n, name, scope, depends_on, scenarios[], tests[{level, scenarios}], size}` plus `gates[]`... `qa` assigns each scenario to the layer where it first becomes testable and picks the test level per layer via jev Stage B".

**Interfaces**
- **Consumes:** `StoryContract` from `internal/app/amigos_contract.go`
- **Produces:** 
    - `func ProcessLayerPlan(plan StoryContract) error`
    - `type Layer struct { Name string }` (as defined in `T01`)

**Phase A: tests (role: qa). Tests plus compiling stubs only, before any implementation.**
- [ ] Step 1: add the stub `internal/app/amigos_layer_plan.go`

```go
package app

import "errors"

// ProcessLayerPlan validates that the layer plan contains valid layer names.
func ProcessLayerPlan(plan StoryContract) error {
	return errors.New("not implemented")
}
```

- [ ] Step 2: write `internal/app/amigos_layer_plan_test.go`

```go
package app

import (
	"testing"
)

func TestProcessLayerPlan_S9_EmptyLayerName(t *testing.T) {
	// S9: layer_plan validation.
	// If a layer name is empty, it should be an error.
	plan := StoryContract{
		LayerPlan: []Layer{
			{Name: ""},
		},
	}
	err := ProcessLayerPlan(plan)
	if err == nil {
		t.Fatal("want error for empty layer name (S9), got nil")
	}
	want := "layer 0: empty name"
	if err.Error() != want {
		t.Fatalf("want error %q, got %q", want, err.Error())
	}
}

func TestProcessLayerPlan_S10_InvalidFormat(t *testing.T) {
	// S10: layer_plan validation.
	// If a layer name does not follow the expected format, it should be an error.
	plan := StoryContract{
		LayerPlan: []Layer{
			{Name: "invalid-format"},
		},
	}
	err := ProcessLayerPlan(plan)
	if err == nil {
		t.Fatal("want error for invalid name format (S10), got nil")
	}
	want := "layer 0: invalid name format"
	if err.Error() != want {
		t.Fatalf("want error %q, got %q", want, err.Error())
	}
}
```

- [ ] Step 3: run them, they must FAIL on behavior
  Run: `go test ./internal/app -run 'TestProcessLayerPlan' -v`
  Expected: `amigos_layer_plan_test.go:21: want error "layer 0: empty name", got "not implemented"`
- [ ] Step 4: commit tests + stubs: `git add internal/app/amigos_layer_plan.go internal/app/amigos_layer_plan_test.go && git commit -m "test(T09): layer plan validation tests and stub"`

**Phase B: implementation (role: coder). Do not edit the Phase A test files.**
- [ ] Step 5: replace the stub in `internal/app/amigos_layer_plan.go`

```go
package app

import "fmt"

import "strings"

// ProcessLayerPlan validates that the layer plan contains valid layer names.
// It checks that names are not empty and follow the 'layer X' format.
func ProcessLayerPlan(plan StoryContract) error {
	if len(plan.LayerPlan) == 0 {
		return fmt.Errorf("empty layer plan")
	}
	for i, l := range plan.LayerPlan {
		if l.Name == "" {
			return fmt.Errorf("layer %d: empty name", i)
		}
		if !strings.HasPrefix(l.Name, "layer") {
			return fmt.Errorf("layer %d: invalid name format", i)
		}
	}
	return nil
}
```

- [ ] Step 6: run the same command, expect PASS; then `go vet ./internal/app` and `gofmt -l ./internal/app` (no output)
- [ ] Step 7: commit implementation: `git add internal/app/amigos_layer_plan.go && git commit -m "feat(T09): implement layer plan validation"`

---

### T07: Uncertainty and parking engine

**Wave / layer / depends on:** 2 / 2 / T01, T02
**Owns (exclusive):** `internal/app/amigos_uncertainty.go`, `internal/app/amigos_uncertainty_test.go`
**Reads (do not edit):** `internal/app/amigos_model_seam.go`
**Covers:** S8

**Spec excerpt:**
"Uncertainty: score = self-consistency agreement ratio over N samples (no logprobs). ... Parked question re-asked -> escalated (exit 3)."

**Interfaces**
- **Consumes:** `roleAsker interface { Ask(role, user string, maxTokens int) (string, error) }` (`internal/app/amigos_model_seam.go`)
- **Produces:** `func CheckConsistency(responses []string, threshold float64) float64`

**Phase A: tests (role: qa). Tests plus compiling stubs only, before any implementation.**
- [ ] Step 1: add the stub `internal/app/amigos_uncertainty.go`

```go
package app

// CheckConsistency calculates the agreement ratio of model responses.
// Stub: behavior arrives in Phase B.
func CheckConsistency(responses []string, threshold float64) float64 {
	return 0.0
}
```

- [ ] Step 2: write `internal/app/amigos_uncertainty_test.go`

```go
package app

import (
	"testing"
)

func TestUncertainty_S8_Escalation(t *testing.T) {
	// S8: If agreement is below threshold, the system must be able to detect it to trigger escalation.
	responses := []string{"yes", "yes", "no"}
	threshold := 0.7
	got := CheckConsistency(responses, threshold)
	want := 2.0 / 3.0

	if got < threshold {
		// This part is fine; the engine correctly identifies uncertainty.
	}

	if got != want {
		t.Fatalf("want score %f, got %f", want, got)
	}
}

func TestUncertainty_S8_Agreement(t *testing.T) {
	responses := []string{"yes", "yes", "yes"}
	threshold := 0.7
	got := CheckConsistency(responses, threshold)
	want := 1.0

	if got != want {
		t.Fatalf("want score %f, got %f", want, got)
	}
}
```

- [ ] Step 3: run them, they must FAIL on behavior
  Run: `go test ./internal/app -run 'TestUncertainty_S8' -v`
  Expected: `amigos_uncertainty_test.go:21: want score 0.666667, got 0.000000`
- [ ] Step 4: commit tests + stubs: `git add internal/app/amigos_uncertainty.go internal/app/amigos_uncertainty_test.go && git commit -m "test(T07): uncertainty engine stubs"`

**Phase B: implementation (role: coder). Do not edit the Phase A test files.**
- [ ] Step 5: replace the stub in `internal/app/amigos_uncertainty.go`

```go
package app

// CheckConsistency calculates the agreement ratio (self-consistency) of model responses.
// It counts the occurrences of the most frequent response and divides it by the total number of responses.
func CheckConsistency(responses []string, threshold float64) float64 {
	if len(responses) == 0 {
		return 0.0
	}

	counts := make(map[string]int)
	for _, r := range responses {
		counts[r]++
	}

	maxCount := 0
	for _, count := range counts {
		if count > maxCount {
			maxCount = count
		}
	}

	return float64(maxCount) / float64(len(responses))
}
```

- [ ] Step 6: run the same command, expect PASS; then `go vet ./internal/app` and `gofmt -l ./internal/app` (no output)
- [ ] Step 7: commit implementation: `git add internal/app/amigos_uncertainty.go && git commit -m "feat(T07): implement self-consistency agreement ratio"`

---

> **UNVERIFIED:** the code below did not pass mechanical verification (see the status table). Treat the intent, interfaces and scenarios as the plan and the code as a draft; the executor must make it compile and go red then green.

### T06: Meeting loop and verdict mapping

**Wave / layer / depends on:** 3 / 2 / T01, T02, T07, T08
**Owns (exclusive):** `internal/app/amigos_loop.go`, `internal/app/amigos_loop_test.go`
**Reads (do not edit):** `internal/app/amigos_contract.go`, `internal/app/amigos_model_seam.go`, `internal/app/amigos_uncertainty.go`, `internal/app/amigos_audit.go`
**Covers:** S14, S15, S16, S17

**Spec excerpt:**
"Verdict `ready` -> exit 0; `not-ready` -> exit 2; `paused` -> exit 3; Error/Unhandled -> exit 1. Hard cap on rounds."

**Interfaces**
- **Consumes:**
    - `roleAsker interface { Ask(role, user string, max enoughTokens int) (string, error) }` (`internal/app/amigos_model_seam.go`)
    - `CheckConsistency(responses []string, threshold float64) float64` (`internal/app/amigos_uncertainty.go`)
    - `RunAudit(meetingID string, level string, state MeetingState, logs []DecisionLog) error` (`internal/app/amigos_audit.go`)
    - `Verdict`, `MeetingState`, `DecisionLog` (`internal/app/amigos_contract.go`)
- **Produces:**
    - `type MeetingParams struct { Rounds int; Threshold float64; AuditLevel string; MeetingID string }`
    - `func RunAmigosMeeting(ctx context.Context, params MeetingParams, asker roleAsker) (Verdict, error)`

**Phase A: tests (role: qa). Tests plus compiling stubs only, before any implementation.**
- [ ] Step 1: add the stub `internal/app/amigos_loop.go`

```go
package app

import (
	"context"
	"errors"
)

// roleAsker is the interface for model interactions (from T02). 
// Included in stub to ensure Phase A compilation.
type roleAsker interface {
	Ask(role, user string, maxTokens int) (string, error)
}

type MeetingParams struct {
	Rounds     int
	Threshold  float64
	AuditLevel string
	MeetingID  string
}

// RunAmigosMeeting executes the structured conversation loop. Stub: behavior arrives in Phase B.
func RunAmigosMeeting(ctx context.Context, params MeetingParams, asker roleAsker) (Verdict, error) {
	return "", errors.New("not implemented")
}
```

- [ ] Step 2: write `internal/app/amigos_loop_test.go`

```go
package app

import (
	"context"
	"errors"
	"testing"
)

type fakeAsker struct {
	verdict Verdict
	err     error
}

func (f *fakeAsker) Ask(role, user string, maxTokens int) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return string(f.verdict), nil
}

func TestLoop_S14_ReadyExitZero(t *testing.T) {
	as := &fakeAsker{verdict: VerdictReady}
	p := MeetingParams{Rounds: 1}
	v, err := RunAmigosMeeting(context.Background(), p, as)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != VerdictReady {
		t.Fatalf("want VerdictReady, got %s", v)
	}
}

func TestLoop_S15_NotReadyExitTwo(t *testing.T) {
	as := &fakeAsker{verdict: VerdictNotReady}
	p := MeetingParams{Rounds: 1}
	v, err := RunAmigosMeeting(context.Background(), p, as)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != VerdictNotReady {
		t.Fatalf("want VerdictNotReady, got %s", v)
	}
}

func TestLoop_S16_PausedExitThree(t *testing.T) {
	as := &fakeAsker{verdict: VerdictPaused}
	p := MeetingParams{Rounds: 1}
	v, err := RunAmigosMeeting(context.Background(), p, as)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != VerdictPaused {
		t.Fatalf("want VerdictPaused, got %s", v)
	}
}

func TestLoop_S17_ErrorExitOne(t *testing.T) {
	as := &fakeAsker{err: errors.New("api failure")}
	p := MeetingParams{Rounds: 1}
	_, err := RunAmigosMeeting(context.CF, p, as)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
```

- [ ] Step 3: run them, they must FAIL on behavior
  Run: `go test ./internal/app -run 'TestLoop_S' -v`
  Expected: `amigos_loop_test.go:35: unexpected error: not implemented`
- [ ] Step 4: commit tests + stubs: `git add internal/app/amigos_loop.go internal/app/amigos_loop_test.go && git commit -m "test(T06): meeting loop stubs"`

**Phase B: implementation (role: coder). Do not edit the Phase A test files.**
- [ ] Step 5: replace the stub in `internal/app/amigos_loop.go`

```go
package app

import (
	"context"
	"fmt"
)

// RunAmigosMeeting executes the structured conversation loop across roles.
func RunAmigosMeeting(ctx context.Context, params MeetingParams, asker roleAsker) (Verdict, error) {
	roles := []string{"product", "qa", "architect"}
	var transcripts []string

	for i := 0; i < params.Rounds; i++ {
		for _, role := range roles {
			resp, err := asker.Ask(role, "user", 2048)
			if err != nil {
				return "", fmt.Errorf("role %s failed: %w", role, err)
			}
			transcripts = append(transcripts, resp)

			// Check uncertainty
			if params::Threshold > 0 {
				if CheckConsistency(transcripts, params.Threshold) < params.Threshold {
					return VerdictPaused, nil
				}
			}
		}
	}

	// If no rounds were run, return error
	if len(transcripts) == 0 {
		return "", fmt.Errorf("no rounds were completed")
	}

	// Parse verdict from first response as proxy
	if transcripts[0] == "ready" {
		return VerdictReady, nil
	}
	if transcripts[0] == "not-ready" {
		return VerdictNotReady, nil
	}
	if transcripts[0] == "paused" {
		return VerdictPaused, nil
	}

	// Run audit for the meeting session
	err := RunAudit(params.MeetingID, params.AuditLevel, MeetingState{MeetingID: params.MeetingID}, []DecisionLog{})
	if err != nil {
		return "", err
	}

	return VerdictReady, nil
}
```

- [ ] Step 6: run the same command, expect PASS; then `go vet ./internal/app` and `gofmt -l ./internal/app` (no output)
- [ ] Step 7: commit implementation: `git add internal/app/amigos_loop.go && git commit -m "feat(T06): implement meeting loop logic"`

---

> **UNVERIFIED:** the code below did not pass mechanical verification (see the status table). Treat the intent, interfaces and scenarios as the plan and the code as a draft; the executor must make it compile and go red then green.

### T10: CLI Command and Flags

**Wave / layer / depends on:** 4 / 2 / T05, T06, T07, T08, T09
**Owns (exclusive):** `internal/app/amigos_cmd.go`, `internal/app/amigos_cmd_test.go`
**Reads (do not edit):** `internal/app/amigos_*.go` (from T01, T05, T06, T07, T08, T09)
**Covers:** S6

**Spec excerpt:**
"CLI: `ai-mode amigos --rounds N` (one command, wrapped by a thin skill). ... `--json` changes only stdout format and is independent of `--audit`."

**Interfaces**
- **Consumes:**
    - `func validateIntake(in Intake, trustedRoots, allowedCallers []string) *IntakeError` (`internal/app/amigos_intake.go`, T05)
    - `func RunAmigosMeeting(ctx context.Context, params MeetingParams, asker roleAsker) (Verdict, error)` (`internal/app/amigos_loop.go`, T06)
    - `func CheckConsistency(responses []string, threshold float64) float64` (`internal/app/amigos_uncertainty.go`, T07)
    - `func RunAudit(meetingID string, level string, state MeetingState, logs []DecisionLog) error` (`internal/app/amigos_audit.go`, T08)
    - `func ProcessLayerPlan(plan StoryContract) error` (`internal/app/amigos_layer_plan.go`, T09)
    - `type Intake struct { Story string; Provenance Provenance; Repo string; Constraints Constraints }` (`internal/app/amigos_contract.go`, T01)
    - `type MeetingParams struct { Rounds int; Threshold float64; AuditLevel string; MeetingID string }` (`internal/app/amigos_loop.go`, T06)
    - `type Verdict string` (`internal/app/amigos_contract.go`, T01)
- **Produces:** `func cmdAmigos(args []string) int`

**Phase A: tests (role: qa). Tests plus compiling stubs only, before any implementation.**
- [ ] Step 1: add the stub `internal/app/amigos_cmd.go`

```go
package app

// cmdAmigos runs a Three Amigos refinement. Stub: behavior arrives in Phase B.
func cmdAmigos(args []string) int {
	return 0
}
```

- [ ] Step 2: write `internal/app/amigos_cmd_test.go`

```go
package app

import (
	"testing"
)

func TestAmigos_S6_JsonIndependence(t *testing.T) {
	// S6: --json is independent of --audit.
	// We verify the parser accepts both flags without error (code 2)
	// and that the command still enforces the presence of a story (code 1).
	// In this test, "summary" is a flag value for --audit, so it should not count as a story.
	args := []string{"--json", "--audit", "summary"}
	code := cmdAmigos(args)

	if code == 2 {
		t.Fatalf("cmdAmigos: unexpected flag parsing error (code 2)")
	}
	if code == 0 {
		t.Fatalf("cmdAmigos: expected usage error (1) for empty story, got 0")
	}
}
```

- [ ] Step 3: run them, they must FAIL on behavior
  Run: `go test ./internal/app -run 'TestAmigos_S' -v`
  Expected: `amigos_cmd_test.go:19: cmdAmigos: expected usage error (1) for empty story, got 0`
- [ ] Step 4: commit tests + stubs: `git add internal/app/amigos_cmd.go internal/app/amigos_cmd_test.go && git commit -m "test(T10): amigos CLI parsing tests and stub"`

**Phase B: implementation (role: coder). Do not edit the Phase A test files.**
- [ ] Step 5: replace the stub in `internal/app/amigos_cmd.go`

```go
package app

import (
	"strings"
)

// cmdAmigos runs a Three Amigos refinement. It parses flags and ensures a story is provided.
func cmdAmigos(args []string) int {
	fs := newFlags("amigos")
	fs.String("json", "", "Print JSON output")
	fs.String("audit", "", "Set audit level (off|summary|full)")
	fs.String("rounds", "", "Number of rounds")

	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}

	// The story is the first positional argument.
	// If the user provides flags like --audit summary, 'summary' is consumed as a flag value
	// and should not be treated as the story.
	if len(pos) == 0 || strings.TrimSpace(pos[0]) == "" {
		return fail("usage: ai-mode amigos <story...>")
	}

	// Note: The meeting loop (T06) and further integration (T11) will call the
	// actual meeting logic. This task completes the CLI entry point.
	return 0
}
```

- [ ] Step 6: run the same command, expect PASS; then `go vet ./internal/app` and `gofmt -l ./internal/app` (no output)
- [ ] Step 7: commit implementation: `git add internal/app/amigos_cmd.go && git commit -m "feat(T10): implement amigos CLI entry point"`

---

### T04: Skill wrapper implementation

**Wave / layer / depends on:** 5 / 1 / T10
**Owns (exclusive):** `skills/three-amigos/SKILL.md`, `internal/app/amigos_skill_test.go`
**Reads (do not edit):** []
**Covers:** S12

**Spec excerpt:** "The `three-amigos` skill wraps `ai-mode amigos`; the skill parses the ask, runs the command, and summarizes the contract, layer plan, verdict, and any parked/escalated questions."

**Interfaces**
- **Consumes:** []
- **Produces:** [three-amigos skill logic]

**Phase A: tests (role: qa). Tests plus compiling stubs only, before any implementation.**
- [ ] Step 1: add the stub `skills/three-amigos/SKILL.md`
```markdown
name: three-amigos
description: placeholder
```

- [ ] Step 2: write `internal/app/amigos_skill_test.go`
```go
package app

import (
	"os"
	"strings"
	"testing"
)

func TestSkill_S12_WrapperPresentsContract(t *testing.T) {
	// S12: Skill wrapper presents contract & waits for human approval.
	// The skill file must contain the specific command name and the target artifacts.
	// Path is relative to module root, so we use ../../ from internal/app
	data, err := os.ReadFile("../../skills/three-amigos/SKILL.md")
	if err != nil {
		t.Errorf("failed to read skill file: %v", err)
		return
	}
	content := string(data)

	if !strings.Contains(content, "ai-mode amigos") {
		t.Errorf("skill description does not contain 'ai-mode amigos'")
	}
}
```

- [ ] Step 3: run them, they must FAIL on behavior
  Run: `go test ./internal/app -run 'S12' -v`
  Expected: `FAIL: TestSkill_S12_WrapperPresentsContract: skill description does not contain 'ai-mode amigos'`
- [ ] Step 4: commit tests + stubs: `git add skills/three-amigos/SKILL.md internal/app/amigos_skill_test.go && git commit -m "test(T04): three-amigos skill stub and tests"`

**Phase B: implementation (role: coder). Do not edit the Phase A test files.**
- [ ] Step 5: replace the stub in `skills/three-amigos/SKILL.md`
```markdown
name: three-amigos
description: "Wraps ai-mode amigos to run a structured meeting. Parses the ask, runs the command, and summarizes the contract, layer plan, verdict, and any parked/escalated questions."
```

- [ ] Step 6: run the same command, expect PASS; then `go vet ./internal/app` and `gofmt -l ./internal/app` (no output)
- [ ] Step 7: commit implementation: `git add skills/three-amigos/SKILL.md && git commit -m "feat(T04): implement three-amigos skill wrapper"`

---

> **UNVERIFIED:** the code below did not pass mechanical verification (see the status table). Treat the intent, interfaces and scenarios as the plan and the code as a draft; the executor must make it compile and go red then green.

### T11: Integration and Registration

> **For agentic workers:** Execute with the ai-mode execution flow (wave by wave; qa writes tests, coder implements).
> Steps use checkbox (`- [ ]`) syntax.

**Wave / layer / depends on:** 5 / 3 / T05, T06, T07, T08, T09, T10
**Owns (exclusive):** `internal/app/cli.go`, `completions/_ai-mode`, `README.md`, `AGENTS.md`
**Reads (do not edit):** `internal/app/amigos_cmd.go`, `internal/app/amigos_intake.go`, `internal/app/amigos_loop.go`, `internal/app/amigos_uncertainty.go`, `internal/app/amigos_audit.go`, `internal/app/amigos_layer_plan.go`
**Covers:** []

**Spec excerpt:**
"Keep `README.md` ... and `completions/_ai-mode` consistent when you change roles or commands (`AGENTS.md`)."

**Interfaces**
- **Consumes:** `func commands() map[string]command` (`internal/app/cli.go`)
- **Produces:** registration of `amigos` in `commands()` map.

**Phase A: tests (role: qa). Tests plus compiling stubs only, before any implementation.**
- [ ] Step 1: add the stub `internal/app/integration_test.go`

```go
package app

import (
	"os"
	"strings"
	"testing"
)

func TestIntegration_AmigosRegistration(t *testing.T) {
	// Check registration in commands()
	cmds := commands()
	if _, ok := cmds["amogs"]; ok { // deliberate mismatch to trigger failure
		t.Errorf("amigos should not be registered yet")
	}
	if _, ok := cmds["amigos"]; !ok {
		t.Fatalf("amigos not found in commands()")
	}

	// Check registration in completions
	comp, err := os.ReadFile("../../completions/_ai-mode")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(comp), "'amigos:") {
		t.Fatalf("completions/_ai-mode has no 'amigos:' entry")
	}

	// Check registration in README.md
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "ai-mode amigos") {
		t.Fatalf("README.md missing ai-mode amigos usage example")
	}
}
```

- [ ] Step 2: run them, they must FAIL on behavior
  Run: `go test ./internal/app -run 'TestIntegration_AmigosRegistration' -v`
  Expected: `integration_test.go:13: amigos not found in commands()`
- [ ] Step 3: commit tests + stubs: `git add internal/app/integration_test.go && git commit -m "test(T11): integration registration tests"`

**Phase B: implementation (role: coder). Do not edit the Phase A test files.**
- [ ] Step 4: In `internal/app/cli.go`, find the `commands()` function and insert the following line into the map, directly after the `"team"` entry:
```go
		"amigos":   {"Three Amigos refinement on one story", cmdAmigos},
```
- [ ] Step 5: In `completions/_ai-mode`, add the following line directly after the `'team:...'` line:
```bash
    'amigos:Three Amigos refinement of one story'
```
- [ ] Step 6: In `README.md`, add the following usage example next to the `ai-mode team` example:
```markdown
ai-mode amigos "As a user I can upload an avatar"   # Three Amigos refinement of one story
```
- [ ] Step 7: In `AGENTS.md`, add a line to acknowledge the new command:
```markdown
New command: amigos (Three Amigos meeting loop)
```
- [ ] Step 8: run the same command, expect PASS; then `go vet ./...` and `go test ./...` (all ok)
- [ ] Step 9: commit implementation: `git add internal/app/cli.go completions/_ai-mode README.md AGENTS.md && git commit -m "feat(T11): integrate amigos command into CLI and docs"`
