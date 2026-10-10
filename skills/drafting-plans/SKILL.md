---
name: drafting-plans
description: "Write the implementation plan for an approved spec or Story Contract. Use whenever the human says draft/write/make the plan, after spec-specialist-review is merged and re-approved, or when chief hands planning to architect, even if they just say 'plan this'. Produces contract-first boundaries and a graph of self-contained task packets with exclusive file ownership, so qa and coder can work in parallel without collisions, and checks the plan with `ai-mode plan lint` and `ai-mode plan verify` before anyone sees it. Use this instead of superpowers:writing-plans for ai-mode work. Runs on architect, never chief."
---

# Drafting plans (ai-mode)

Derived from `superpowers:writing-plans` (bite-sized steps, no placeholders, TDD), changed for our setup:
**architect plans, qa and coder execute, executors are small local models, work should run in parallel without
colliding, and the architect has no compiler.** That last fact shapes the whole skill: whatever code architect writes is
checked by a tool, and most code is left to the executors who can run it. Run this on **architect** (262k); chief must not
write plans.

Announce: "I'm using the ai-mode drafting-plans skill to create the implementation plan."

<HARD-GATE>
- Input is an **approved** spec and, when one exists, its Story Contract and `layer_plan[]`. Otherwise stop and ask.
- Before handing the plan to anyone: `ai-mode plan lint <plan>` exits 0 **and** `ai-mode plan verify <plan> --task all`
  exits 0 (a task reported `n/a` is fine; `failed` is not). Quote both results in the plan header.
- No placeholders (TBD, TODO, "add appropriate error handling", "similar to Task N", "write tests for the above").
- Planning only: the plan file is the only thing you write. No production code, no pushes; do not commit the plan unless
  the human asks.
</HARD-GATE>

## The four rules that make parallel work possible

1. **Tasks never rely on each other's implementation.** A task's tests may use only (a) the repo as it is today, (b)
   **contract tasks** it depends on (interfaces, types, fixtures, fakes), and (c) its own code. So an `impl` task may
   depend only on `contract` tasks. If the loop must call an uncertainty engine, wave 0 defines the interface
   (`type uncertaintyEngine interface {...}`) and a fake, the loop is built and tested against the fake, and an
   `integration` task wires the real engine in. `ai-mode plan lint` rejects an impl task that depends on another impl task.
2. **Contracts first.** Every seam gets a contract task: Go interfaces or types, recorded fixtures, an in-process fake,
   and for external I/O (HTTP, process, file system) an injected function or interface plus an `httptest`-style fake. Code
   that needs a live server, the active profile or other global state is not testable and must be wrapped behind such a seam.
3. **Exclusive ownership.** Every task lists the paths it owns. Tasks that could run in parallel (neither is the other's
   ancestor) may not overlap. Hot files (`internal/app/cli.go`, `completions/_ai-mode`, `README.md`, `AGENTS.md`, `go.mod`)
   belong to exactly one `integration` task. A task may only write files it owns.
4. **Tests carry the story.** Every Story Contract scenario S<n> is covered by exactly one task, and the test name contains
   its id (`Test<Area>_S<n>_<Slug>`). Tests are written first and must be seen failing for the right reason.

## Who writes the code

| Packet kind | Used for | What the packet contains | Checked by |
| --- | --- | --- | --- |
| **code** | `contract` tasks (always), seam adapters, simple wiring in `integration` tasks | Complete, runnable code in file blocks (Phase A stub + tests, Phase B implementation) | `ai-mode plan verify` (compile, gofmt, vet, red then green, full gate) |
| **spec** | everything else (`impl`, `docs`) | Owned files, exact interfaces consumed/produced, each scenario as Given/When/Then with its test name, which fakes to use, the run command and the expected red reason | `ai-mode plan lint`; qa and coder write the code with a compiler, then `tdd-check` gates it |

Why: architect cannot compile or run tests, and in practice about half of its long code blocks fail to build. qa and
coder run the compiler. You may put code in a spec packet, but then mark it `packet: code` so `plan verify` checks it; do
not leave unchecked code in a plan.

## Process (staged, with the tools in the loop)

1. **Read** the spec, Story Contract and `layer_plan[]`; list the scenario ids.
2. **Get repo context** (see below); map the existing boundaries, patterns, test style and hot files.
3. **Stage 1, the skeleton** (this is where plans go wrong: lint can check a graph but not judge a decomposition, so have chief or the human read the task list and scenarios before Stage 2; in the first real run architect on its own produced a bundled "adapters" task, thin scenarios and interfaces declared in a test file): header, Global Constraints, Review Focus, Boundaries, the JSON task graph, and the scenario
   coverage table. Run `ai-mode plan lint` (it prints the computed waves) and fix every ERR before going on. Do not write
   `wave` numbers; the tool computes them.
4. **Stage 2, packets in dependency order:** contract tasks first. After each **code** packet run
   `ai-mode plan verify <plan> --task Tnn`. On `failed`, read the failing step, fix the root cause (not the symptom) and
   re-run. After 4 failed attempts do not leave unchecked code in the plan: downgrade the task to a **spec packet** (change `packet` to `spec` in the graph) or escalate to the human. Later packets copy signatures from the
   verified packets they depend on.
5. **Finish:** `plan lint` and `plan verify --task all` both clean; map tasks to stack layers (`skills/pr-stack`).

## Plan document

````markdown
# [Feature] Implementation Plan

> **For agentic workers:** execute wave by wave; qa writes tests, coder implements. Steps use `- [ ]` checkboxes.

**Goal:** one sentence
**Architecture:** 2-3 sentences
**Tech Stack:** key technologies
**Spec:** path    **Story Contract:** path (or "none")
**Context:** tools | digest | spec only        (what architect actually had; see below)
**Lint:** `ai-mode plan lint` exit 0 · **Verify:** `ai-mode plan verify --task all` exit 0

## Global Constraints
Project-wide requirements copied verbatim from the spec, one line each.

## Review Focus
Five input classes or failure modes the spec implies but no task tests yet, most likely to bite first; each names the
scenario id that pins it.

## Boundaries
| Boundary | Contract artifact | Fixtures | Fake | Provider check | Owner task |
| --- | --- | --- | --- | --- | --- |

## Task graph

```json
{"tasks":[
 {"id":"T01","title":"Contracts","kind":"contract","packet":"code","depends_on":[],
  "owns":["internal/app/x_contract.go"],"produces":["type Thing"],"consumes":[],"covers":[],
  "test_level":"contract","stack_layer":1,"branch":"feat-01-contracts-t01"},
 {"id":"T02","title":"Logic","kind":"impl","packet":"spec","depends_on":["T01"],
  "owns":["internal/app/x_logic.go","internal/app/x_logic_test.go"],"produces":["func Do() error"],
  "consumes":["Thing"],"covers":["S1"],"test_level":"unit","stack_layer":2,"branch":"feat-02-logic-t02"}
]}
```

**Scenario coverage**

| Scenario | Rule | Task | Test |
| --- | --- | --- | --- |
| S1 | one-line rule | T02 | `TestLogic_S1_RejectsEmpty` |
````

`kind` is `contract | impl | integration | docs`; `packet` is `code | spec`. Optional `"hot_files": [...]` in the graph overrides
the default hot-file list.

## Task packets

The `**Spec excerpt:**` line contains only quotes copied from the spec, joined by ` and `, with `...` allowed to elide text inside a quote (or the word `none`); commentary goes on a separate `**Spec context:**` line. `ai-mode plan lint` checks every quoted fragment against the spec named in the `**Spec:**` header, so a paraphrase fails.

Every packet starts with `### Tnn: title` and is complete on its own: the implementer sees only their packet and the Global
Constraints. See `references/example-plan.md` for a full worked plan whose code was checked with these tools.

**File blocks.** A fenced block that writes a file says so in its info string:

````
```go file=internal/app/x_logic.go
```
```text file=presets/amigos-callers.list
```
```go file=internal/app/cli.go mode=insert-after anchor="cmdTeam}"
```
````

`mode` is `create` (default; whole file), `append`, `insert-after` (after the first line containing `anchor`), or `replace-line`
(replaces that line). Editing an existing file always uses a mode with an anchor, never a whole-file rewrite of a file you did
not see in full. A block may only write paths its task owns.

**Code packet:** header fields (Owns, Reads, Covers, Spec excerpt, Interfaces consumed/produced), then
`**Phase A: tests plus stubs (role: qa).**` with the stub block(s) and test block(s), `Run: \`command\`` and the expected
failure text (an assertion or "not implemented", never a build error), then `**Phase B: implementation (role: coder).**` with the
implementation block(s). Contract packets have no red step; they need only compile, gofmt, vet and the full gate.

**Spec packet:** the same header fields, then for each scenario a Given/When/Then, the test name, the fake to use and the file
it lives in; `Run: \`command\``; the red reason; and a note on which collaborators come from contracts. No placeholders: say
exactly what to assert.

## Getting repo context

`ai-mode ask` is one chat call with no file tools: architect only sees what is in the prompt. Either run architect in an
agentic client that can read files (Qwen Code with `/model architect`), or have the caller pass material with
`ai-mode ask architect "..." --context <file>... --context <dir>` (a file's contents, a directory's file listing; over
`--context-max-chars` is an error, so pick the relevant files and the existing test style). State in the plan header which you
had. If you had only the spec, mark every file path and signature **unverified** and do not invent them.

## Self-review (what the tools cannot see)

1. Every spec requirement has a scenario and a task; the Story Contract's scenarios are all in the coverage table.
2. Types and signatures in later packets match the earlier packets that produce them.
3. Each cross-task dependency is a contract (interface + fake), not another task's code.
4. The **protocol between components** is defined somewhere (for example what a role's reply looks like and how it is parsed); tools cannot see a missing protocol, and fakes that "return resolved" without a format hide it.
5. Spec excerpts are copied verbatim (now checked by the lint); the protocol and design decisions the tools cannot see are written down in the plan.
6. Packet sizes fit a small model's context (about 400 lines or 40k characters, at most about 5 owned files); split anything bigger.
7. Review Focus has five items, each pinned by a named test.

## Deterministic checks

- **`ai-mode plan lint <plan.md> [--json]`** (exit 0 clean, 2 findings): required sections and Context line; valid JSON graph;
  known dependencies, no cycles; impl tasks depend only on contract tasks; contract tasks are code packets; no ownership
  overlap between tasks that can run in parallel; hot files only in one integration task; every scenario covered by exactly one
  task and test names carry the scenario id; each packet mentions the exact test name the coverage table promises for its scenarios; every quoted fragment of a spec excerpt appears in the spec; each task has a packet, code packets have file blocks and a Run line, blocks
  write only owned files; branch names unique; placeholders; consumed names produced by an ancestor (warning). Prints the
  computed waves.
- **`ai-mode plan verify <plan.md> --task Tnn|all [--repo DIR] [--json]`** (exit 0 verified or n/a, 2 failed): on a temporary
  copy of the repo, applies the task's ancestors' code, then Phase A, requires it to compile and the Run command to fail on an
  assertion (not a build error, not "passes", not "no tests to run"), applies Phase B, requires gofmt, `go vet ./...`, the Run
  command green and `go test ./...` green. Contract tasks skip the red step. Every code task also needs `go build ./...` to pass (production code must not depend on declarations that exist only in `_test.go` files). Spec packets report `n/a`; an integration task
  that depends on spec packets reports `n/a` until the executors have written them.
- **`ai-mode plan tdd-check <plan.md> <task-id>`** (planned, not built): on the executors' branch, the test commit must fail
  and the tip must pass, and no later commit may edit the Phase A tests.

## Handoff

After lint and verify are clean: save the plan, link it for the human and say what you need (review, execution choice). Do not
start execution. At the architect -> coder handoff, coder reads the plan and pushes back on gaps; that is the adversarial
review. Plan changes after that go through architect.

## Related

- `skills/pr-stack/SKILL.md`: how layers become a stack of PRs.
- `references/example-plan.md`: worked example. `evals/evals.json`: test prompts.
- `docs/superpowers/specs/2026-10-10-three-amigos-design.md`: Story Contract, layer plan, gates.
