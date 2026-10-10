# Amigos slice 1: intake gate, verdict exit codes, command registration (Implementation Plan)

> **For agentic workers:** execute wave by wave; qa writes tests, coder implements. Steps use `- [ ]` checkboxes.

**Goal:** `ai-mode amigos` exists as a registered command with a tested intake gate and verdict-to-exit-code mapping.
**Architecture:** New flat files in `internal/app/` (the package already keeps one file per concern, e.g. `ask.go`, `team.go`). A contract task defines the shared types first; the gate and the exit-code mapping depend only on that contract, so they are built and tested independently; one integration task owns the shared registration files.
**Tech Stack:** Go 1.22, standard library only.
**Spec:** `docs/superpowers/specs/2026-10-10-three-amigos-design.md`    **Story Contract:** none (the scenario ids below are invented for this example)
**Context:** digest
**Lint:** `ai-mode plan lint` exit 0 · **Verify:** `ai-mode plan verify --task all` exit 0

> This is a worked example for `skills/drafting-plans`. It is a regression fixture: `internal/app/plan_example_test.go` lints it on every `go test`, and `make plan-example` verifies its code tasks against this repository, so it cannot drift from the tools. T03 is a spec packet on purpose, to show that form.

## Global Constraints
- Go standard library only; module `ai-mode`, `go 1.22` (`go.mod`).
- Match the package's style: flat files in `internal/app/`, tests in `package app`, `t.Fatalf` assertions (`team_test.go`).
- Do not invent CLI flags; confirm against `internal/app/` (`AGENTS.md`).
- Keep `completions/_ai-mode` and `README.md` consistent when a command is added (`AGENTS.md`).
- Local commits on task branches are authorized by running this plan; no pushes (`skills/pr-stack/SKILL.md`).

## Review Focus
1. A whitespace-only story must be a `schema_mismatch`, not accepted (S9).
2. A repo path that does not exist must be `path_outside_roots`, not a pass (S10).
3. A repo equal to a trusted root must be accepted (S1, second assertion).
4. `sk-` style tokens count as secrets, not only `tk_` (S11).
5. An unauthorized caller is reported before a secret in the story (S11).

## Boundaries
| Boundary | Contract artifact | Fixtures | Fake | Provider check | Owner task |
| --- | --- | --- | --- | --- | --- |
| Intake request/response shape between callers and the gate | `internal/app/amigos_contract.go` (`Intake`, `IntakeError`, `Verdict`, error codes) | none (pure Go values) | none needed (in-process) | `amigos_intake_test.go` | T01 |

## Task graph

```json
{"tasks":[
 {"id":"T01","title":"Contract types for the amigos command","kind":"contract","packet":"code","depends_on":[],
  "owns":["internal/app/amigos_contract.go"],
  "produces":["type Verdict, Provenance, Intake, IntakeError","const VerdictReady/NotReady/Paused","const ErrUnauthorizedCaller/ErrSchemaMismatch/ErrPathOutsideRoots/ErrSecretDetected"],
  "consumes":[],"covers":[],"test_level":"contract","stack_layer":1,"branch":"amigos-slice-01-contract-t01"},
 {"id":"T02","title":"Intake gate","kind":"impl","packet":"code","depends_on":["T01"],
  "owns":["internal/app/amigos_intake.go","internal/app/amigos_intake_test.go"],
  "produces":["func validateIntake(in Intake, trustedRoots, allowedCallers []string) *IntakeError"],
  "consumes":["Intake","IntakeError","contains(xs []string, s string) bool (internal/app/cmds.go)"],
  "covers":["S1","S2","S3","S9","S10","S11"],"test_level":"unit","stack_layer":2,"branch":"amigos-slice-02-gate-t02"},
 {"id":"T03","title":"Verdict exit codes and first output line","kind":"impl","packet":"spec","depends_on":["T01"],
  "owns":["internal/app/amigos_render.go","internal/app/amigos_render_test.go"],
  "produces":["func exitCodeFor(v Verdict) int","func verdictLine(v Verdict) string"],
  "consumes":["Verdict"],"covers":["S4","S5","S6"],"test_level":"unit","stack_layer":2,"branch":"amigos-slice-02-render-t03"},
 {"id":"T04","title":"Register the command (integration task, owns the shared files)","kind":"integration","packet":"code","depends_on":["T01","T02"],
  "owns":["internal/app/amigos_cmd.go","internal/app/amigos_cmd_test.go","internal/app/cli.go","completions/_ai-mode","README.md"],
  "produces":["func cmdAmigos(args []string) int","commands()[\"amigos\"]"],
  "consumes":["newFlags, parse, fail (internal/app/cli.go)"],"covers":["S7","S8"],"test_level":"functional","stack_layer":3,"branch":"amigos-slice-03-register-t04"}
]}
```

Waves are computed by `ai-mode plan lint`: T01 is wave 0, T02 and T03 are wave 1 (parallel-safe: disjoint files, both consume only T01's types), T04 is wave 2.

**Scenario coverage**

| Scenario | Rule | Task | Test |
| --- | --- | --- | --- |
| S1 | A caller not on the allowlist is rejected; an allowlisted one is accepted | T02 | `TestIntake_S1_UnauthorizedCaller` |
| S2 | A repo that resolves (through a symlink or a URL) outside trusted roots is rejected | T02 | `TestIntake_S2_RepoSymlinkEscapeRejected` |
| S3 | A token-like string in the story is rejected with `secret_detected` | T02 | `TestIntake_S3_SecretInStoryRejected` |
| S4 | `ready` exits 0 | T03 | `TestExitCode_S4_ReadyIsZero` |
| S5 | `not-ready` exits 2 | T03 | `TestExitCode_S5_NotReadyIsTwo` |
| S6 | `paused` exits 3, an unknown verdict exits 1, the verdict line is `verdict: <v>` | T03 | `TestExitCode_S6_PausedIsThreeAndUnknownIsOne` |
| S7 | `amigos` is in `commands()`, the usage text and the zsh completions | T04 | `TestAmigos_S7_RegisteredInUsageAndCompletions` |
| S8 | `ai-mode amigos` with no story is a usage error (exit 1) | T04 | `TestAmigos_S8_NoStoryIsUsageError` |
| S9 | A blank story is `schema_mismatch` | T02 | `TestIntake_S9_BlankStoryIsSchemaMismatch` |
| S10 | A missing repo path is `path_outside_roots` | T02 | `TestIntake_S10_MissingRepoPathIsOutsideRoots` |
| S11 | `sk-` tokens count; an unauthorized caller is reported before secrets | T02 | `TestIntake_S11_UnauthorizedCallerIsReportedBeforeSecrets` |

---

### T01: Contract types for the amigos command

**Wave / layer / depends on:** 0 / 1 / none
**Owns (exclusive):** `internal/app/amigos_contract.go`
**Covers:** none (a contract has no behavior to go red on; it must compile, be gofmt-clean and keep the full gate green)

**Spec excerpt:** "**Intake contract** (JSON): `story` (text, treated strictly as data), `provenance` (`human|skill|agent` + caller id), `authorization` (vetting reference, required for non-human), optional `repo` path and `constraints`." and "Exit codes: 0 ready, 2 not-ready, 3 paused (needs a human), 1 error."

**Interfaces**
- Consumes: nothing.
- Produces: `Verdict` with `VerdictReady`, `VerdictNotReady`, `VerdictPaused`; `Provenance{Kind, CallerID string}`; `Intake{Story string; Provenance Provenance; Repo string}`; the codes `ErrUnauthorizedCaller`, `ErrSchemaMismatch`, `ErrPathOutsideRoots`, `ErrSecretDetected`; `IntakeError{Code, Detail string}` with `Error() string`.

**Phase A: the types.**
- [ ] Step 1: create `internal/app/amigos_contract.go`

```go file=internal/app/amigos_contract.go
package app

// Verdict is the outcome of an amigos meeting.
type Verdict string

const (
	VerdictReady    Verdict = "ready"
	VerdictNotReady Verdict = "not-ready"
	VerdictPaused   Verdict = "paused"
)

// Provenance says who submitted a story.
type Provenance struct {
	Kind     string `json:"kind"` // human | skill | agent
	CallerID string `json:"caller_id"`
}

// Intake is the request contract for `ai-mode amigos`.
type Intake struct {
	Story      string     `json:"story"`
	Provenance Provenance `json:"provenance"`
	Repo       string     `json:"repo,omitempty"`
}

// Codes returned by the intake gate.
const (
	ErrUnauthorizedCaller = "unauthorized_caller"
	ErrSchemaMismatch     = "schema_mismatch"
	ErrPathOutsideRoots   = "path_outside_roots"
	ErrSecretDetected     = "secret_detected"
)

// IntakeError is the structured error the intake gate returns.
type IntakeError struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

func (e *IntakeError) Error() string { return e.Code + ": " + e.Detail }
```

Run: `go vet ./...`

**Phase B: nothing more.** The full gate is `gofmt -l internal` (no output), `go vet ./...`, `go test ./...`.

---

### T02: Intake gate

**Wave / layer / depends on:** 1 / 2 / T01
**Owns (exclusive):** `internal/app/amigos_intake.go`, `internal/app/amigos_intake_test.go`
**Reads (do not edit):** `internal/app/amigos_contract.go`, `internal/app/cmds.go` (for `contains`)
**Covers:** S1, S2, S3, S9, S10, S11

**Spec excerpt:** "**Intake gate** runs before any model call: strict schema validation; `repo` must be a local path: symlinks are resolved (EvalSymlinks) and the canonical path must be a descendant of a `trusted_roots` entry (`..`, URLs and other schemes rejected, so no remote fetch and no symlink escape to files like `~/.ssh` during repo mapping); `caller_id` must be in the caller allowlist, else exit 1; a secret scan (`tk_`, `sk_`, known token patterns) over story text." and "**Intake errors are structured:** a validation failure returns exit 1 and a JSON error `{code, detail}` with `code` one of `unauthorized_caller`, `schema_mismatch`, `path_outside_roots`, `secret_detected`, so calling skills can react programmatically."

**Interfaces**
- Consumes: `Intake`, `Provenance`, `IntakeError` and the `Err*` constants (T01, `internal/app/amigos_contract.go`); `func contains(xs []string, s string) bool` (`internal/app/cmds.go`).
- Produces: `func validateIntake(in Intake, trustedRoots, allowedCallers []string) *IntakeError` (nil means accepted). Check order: blank story, caller allowlist, repo, secrets.

**Phase A: tests plus stubs (role: qa). No behavior yet.**
- [ ] Step 1: add the stub

```go file=internal/app/amigos_intake.go
package app

// validateIntake is the gate that runs before any model call. Stub: behavior arrives in Phase B.
func validateIntake(in Intake, trustedRoots, allowedCallers []string) *IntakeError {
	return &IntakeError{Code: "not_implemented"}
}
```

- [ ] Step 2: write the tests

```go file=internal/app/amigos_intake_test.go
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
		Provenance: Provenance{Kind: "skill", CallerID: "brainstorming"},
		Repo:       repo,
	}
}

func wantCode(t *testing.T, got *IntakeError, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Fatalf("want accepted, got %s", got)
		}
		return
	}
	if got == nil || got.Code != want {
		t.Fatalf("want code %q, got %v", want, got)
	}
}

func TestIntake_S1_UnauthorizedCaller(t *testing.T) {
	root := t.TempDir()
	in := okIntake(root)
	in.Provenance.CallerID = "stranger"
	wantCode(t, validateIntake(in, []string{root}, []string{"brainstorming"}), ErrUnauthorizedCaller)
	wantCode(t, validateIntake(okIntake(root), []string{root}, []string{"brainstorming"}), "")
}

func TestIntake_S2_RepoSymlinkEscapeRejected(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	inside := filepath.Join(root, "repo")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	callers := []string{"brainstorming"}
	wantCode(t, validateIntake(okIntake(link), []string{root}, callers), ErrPathOutsideRoots)
	wantCode(t, validateIntake(okIntake("https://example.com/r.git"), []string{root}, callers), ErrPathOutsideRoots)
	wantCode(t, validateIntake(okIntake(inside), []string{root}, callers), "")
}

func TestIntake_S3_SecretInStoryRejected(t *testing.T) {
	in := okIntake("")
	in.Story = "use token tk_" + strings.Repeat("a1", 12) + " to call the board"
	wantCode(t, validateIntake(in, nil, []string{"brainstorming"}), ErrSecretDetected)
}

func TestIntake_S9_BlankStoryIsSchemaMismatch(t *testing.T) {
	in := okIntake("")
	in.Story = "  \n\t "
	wantCode(t, validateIntake(in, nil, []string{"brainstorming"}), ErrSchemaMismatch)
}

func TestIntake_S10_MissingRepoPathIsOutsideRoots(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "does-not-exist")
	wantCode(t, validateIntake(okIntake(missing), []string{root}, []string{"brainstorming"}), ErrPathOutsideRoots)
}

func TestIntake_S11_UnauthorizedCallerIsReportedBeforeSecrets(t *testing.T) {
	in := okIntake("")
	in.Provenance.CallerID = "stranger"
	in.Story = "token sk-" + strings.Repeat("Z9", 12)
	wantCode(t, validateIntake(in, nil, []string{"brainstorming"}), ErrUnauthorizedCaller)
	in.Provenance.CallerID = "brainstorming"
	wantCode(t, validateIntake(in, nil, []string{"brainstorming"}), ErrSecretDetected)
}
```

- [ ] Step 3: run them, they must FAIL on an assertion, not a build error
  Run: `go test ./internal/app -run TestIntake_S -v`
  Expected: each test fails with a message like `want code "unauthorized_caller", got not_implemented: `
- [ ] Step 4: commit tests + stub: `git add internal/app/amigos_intake.go internal/app/amigos_intake_test.go && git commit -m "test(T02): intake gate tests and stub"`

**Phase B: implementation (role: coder). Do not edit the test file.**
- [ ] Step 5: replace the stub

```go file=internal/app/amigos_intake.go
package app

import (
	"path/filepath"
	"regexp"
	"strings"
)

var secretRe = regexp.MustCompile(`\b(tk_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9]{20,})`)

// validateIntake is the gate that runs before any model call. It returns nil
// when the request may proceed.
func validateIntake(in Intake, trustedRoots, allowedCallers []string) *IntakeError {
	if strings.TrimSpace(in.Story) == "" {
		return &IntakeError{ErrSchemaMismatch, "story is empty"}
	}
	if !contains(allowedCallers, in.Provenance.CallerID) {
		return &IntakeError{ErrUnauthorizedCaller, "caller " + in.Provenance.CallerID + " is not allowlisted"}
	}
	if in.Repo != "" {
		if err := checkRepo(in.Repo, trustedRoots); err != nil {
			return err
		}
	}
	if secretRe.MatchString(in.Story) {
		return &IntakeError{ErrSecretDetected, "story contains a token-like string"}
	}
	return nil
}

// checkRepo requires a local path whose symlink-resolved form sits inside a trusted root.
func checkRepo(repo string, trustedRoots []string) *IntakeError {
	if strings.Contains(repo, "://") {
		return &IntakeError{ErrPathOutsideRoots, "repo must be a local path"}
	}
	resolved, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return &IntakeError{ErrPathOutsideRoots, "repo cannot be resolved: " + err.Error()}
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
	return &IntakeError{ErrPathOutsideRoots, "repo resolves outside the trusted roots"}
}
```

- [ ] Step 6: run the same command, expect PASS (all six tests); then `gofmt -l internal` (no output), `go vet ./...`, `go test ./...`
- [ ] Step 7: commit: `git add internal/app/amigos_intake.go && git commit -m "feat(T02): intake gate"`

**If a test looks wrong:** do not edit it. Write the question in the task thread; architect and qa decide.

---

### T03: Verdict exit codes and first output line

A **spec packet**: qa and coder write the code in their tool loop; `ai-mode plan lint` checks this packet's structure and `tdd-check` gates the branch.

**Wave / layer / depends on:** 1 / 2 / T01
**Owns (exclusive):** `internal/app/amigos_render.go`, `internal/app/amigos_render_test.go`
**Reads (do not edit):** `internal/app/amigos_contract.go`
**Covers:** S4, S5, S6

**Spec excerpt:** "Exit codes: 0 ready, 2 not-ready, 3 paused (needs a human), 1 error." and "Default stdout: verdict line first, then layer plan"

**Interfaces**
- Consumes: `Verdict` and the `Verdict*` constants (T01).
- Produces: `func exitCodeFor(v Verdict) int` and `func verdictLine(v Verdict) string`. Both are pure functions; no fakes are needed.

**Scenarios (write these tests first, in `internal/app/amigos_render_test.go`, package `app`, `t.Fatalf` style):**
- S4 `TestExitCode_S4_ReadyIsZero`: Given `VerdictReady`, when `exitCodeFor` runs, then it returns 0.
- S5 `TestExitCode_S5_NotReadyIsTwo`: Given `VerdictNotReady`, then it returns 2.
- S6 `TestExitCode_S6_PausedIsThreeAndUnknownIsOne`: Given `VerdictPaused`, then 3; given `Verdict("bogus")`, then 1; and `verdictLine(VerdictPaused)` returns exactly `verdict: paused`.

Run: `go test ./internal/app -run TestExitCode_S -v`
**Red reason:** with stubs `exitCodeFor` returning 1 and `verdictLine` returning `""`, S4, S5 and S6 fail on `want exit 0, got 1`-style assertions. Add the stubs first so the tests compile.
**Done when:** the three tests pass, `gofmt -l internal` is empty, and `go vet ./...` and `go test ./...` pass.

---

### T04: Register the command

**Wave / layer / depends on:** 2 / 3 / T01, T02
**Owns (exclusive):** `internal/app/amigos_cmd.go`, `internal/app/amigos_cmd_test.go`, `internal/app/cli.go`, `completions/_ai-mode`, `README.md`
**Reads (do not edit):** `internal/app/amigos_*.go` from T01 and T02
**Covers:** S7, S8

**Spec excerpt:** "CLI: `ai-mode amigos --rounds N` (one command, wrapped by a thin skill"
**Spec context:** In this slice the command only parses a story and rejects an empty one; the meeting loop arrives in a later plan. `AGENTS.md` requires `README.md` and `completions/_ai-mode` to stay consistent when a command is added.

**Interfaces**
- Consumes: `newFlags(name string) *flag.FlagSet`, `parse(fs, args) (pos []string, code int, ok bool)`, `fail(format string, a ...any) int` (`internal/app/cli.go`); `commands() map[string]command` and `usage(w io.Writer)`.
- Produces: `func cmdAmigos(args []string) int` and the `"amigos"` entry in `commands()`.

**Phase A: tests plus a stub (role: qa)**
- [ ] Step 1: add the stub

```go file=internal/app/amigos_cmd.go
package app

// cmdAmigos runs a Three Amigos refinement. Stub: behavior arrives in Phase B.
func cmdAmigos(args []string) int { return 0 }
```

- [ ] Step 2: write the tests

```go file=internal/app/amigos_cmd_test.go
package app

import (
	"os"
	"strings"
	"testing"
)

func TestAmigos_S7_RegisteredInUsageAndCompletions(t *testing.T) {
	if _, ok := commands()["amigos"]; !ok {
		t.Fatal(`"amigos" is not registered in commands()`)
	}
	var b strings.Builder
	usage(&b)
	if !strings.Contains(b.String(), "amigos") {
		t.Fatalf("usage does not list amigos:\n%s", b.String())
	}
	comp, err := os.ReadFile("../../completions/_ai-mode")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(comp), "'amigos:") {
		t.Fatal("completions/_ai-mode has no 'amigos:' entry")
	}
}

func TestAmigos_S8_NoStoryIsUsageError(t *testing.T) {
	if got := cmdAmigos(nil); got != 1 {
		t.Fatalf("no story: want exit 1, got %d", got)
	}
}
```

- [ ] Step 3: run, must FAIL on an assertion
  Run: `go test ./internal/app -run TestAmigos_S -v`
  Expected: `"amigos" is not registered in commands()` and `no story: want exit 1, got 0`
- [ ] Step 4: commit: `git add internal/app/amigos_cmd.go internal/app/amigos_cmd_test.go && git commit -m "test(T04): amigos registration tests and stub"`

**Phase B: implementation (role: coder). Do not edit the test file.**
- [ ] Step 5: replace the stub

```go file=internal/app/amigos_cmd.go
package app

import "strings"

// cmdAmigos runs a Three Amigos refinement on one story. In this slice it only
// parses the story and rejects an empty one; the meeting loop arrives in a later plan.
func cmdAmigos(args []string) int {
	fs := newFlags("amigos")
	pos, code, ok := parse(fs, args)
	if !ok {
		return code
	}
	if strings.TrimSpace(strings.Join(pos, " ")) == "" {
		return fail("usage: ai-mode amigos <story...> (or pipe the story on stdin)")
	}
	return fail("amigos: the meeting loop is not implemented yet")
}
```

- [ ] Step 6: register it in `commands()`, directly after the `"team"` entry

```go file=internal/app/cli.go mode=insert-after anchor="cmdTeam}"
		"amigos":   {"Run a Three Amigos refinement on one story (product, qa, architect)", cmdAmigos},
```

- [ ] Step 7: add it to the zsh completions, directly after the `team` line

```text file=completions/_ai-mode mode=insert-after anchor="'team:Consult"
    'amigos:Three Amigos refinement of one story'
```

- [ ] Step 8: document it in the README, directly after the `ai-mode team` example

```text file=README.md mode=insert-after anchor="web version of the app"
ai-mode amigos "As a user I can upload an avatar"   # Three Amigos refinement of one story (meeting loop not built yet)
```

- [ ] Step 9: run the tests, expect PASS; then the full gate: `gofmt -l internal` (no output), `go vet ./...`, `go test ./...`
- [ ] Step 10: commit: `git add internal/app/amigos_cmd.go internal/app/cli.go completions/_ai-mode README.md && git commit -m "feat(T04): register ai-mode amigos"`

**Integration duties (end of the last wave):** restack the layer branches (`gh stack rebase --upstack`), run the full gates, and confirm `README.md`, `completions/_ai-mode` and `commands()` agree.

---

## What this example shows

- **Contract first:** T01 lands before anything else; T02 and T03 depend only on it and never read each other's files.
- **Parallel by construction:** T02 and T03 share no paths, and the lint proves it.
- **Hot files have one owner:** `cli.go`, `completions/_ai-mode` and `README.md` are touched only by T04, the integration task, through anchored edits to files it did not create.
- **Red is real:** Phase A includes a compiling stub, so tests fail on assertions, never on `undefined:`.
- **Intent is traceable:** every scenario id maps to a task and to a test name containing that id.
- **Code vs spec packets:** T01, T02 and T04 carry code that `plan verify` checks; T03 is a spec packet for qa and coder to implement.
