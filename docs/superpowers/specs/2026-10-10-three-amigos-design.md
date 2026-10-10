# Three Amigos (`ai-mode amigos`) — design notes

Status: **draft, blocked on CLI plumbing.** Captures decisions from the 2026-10-10 discussion. Not yet reviewed via `spec-specialist-review`.

## Goal

A short, structured conversation between **product**, **qa**, and **architect** about one story, held before any code is written. Output is a **Story Contract**: the team's agreed definition of the story (rules, concrete examples tied to rules, open and parked questions, size, split-offs), a verdict that says whether it is "ready" (or why not), and the **layer plan** (stack design plus CI gates) that drives a stacked-PR, TDD delivery. It is the thing the base PR commits and every later layer satisfies. (Formerly called the "Example Map"; Example Mapping stays the facilitation technique used to produce the rules, examples and questions, but the artifact holds more than the map.)

## Shape

- CLI: `ai-mode amigos --rounds N` (one command, wrapped by a thin skill: one skill ↔ one tool call). The skill parses the ask, runs the command, and summarizes the contract, layer plan, verdict, and any parked/escalated questions.
- Roles: `product` (rules, scope) → `qa` (examples, edge cases; jev Stage B picks the `test_layer` for each stack layer) → `architect` (feasibility, size, red questions; 262k context lets it map the repo). Each role reacts to the others; at least two rounds.
- Developer amigo is **architect** (revised 2026-10-10; earlier draft had coder). The same role then owns the plan, so the story's feasibility concerns and the plan share one author.
- **coder is not part of the standing flow or the normal spec review.** It joins a meeting only when the amigos request a full team review (the `ai-mode team` panel); in that case it is added as an extra voice. Adversarial scrutiny of the plan happens naturally at the architect -> coder handoff, when coder tries to execute it and pushes back on gaps; no separate review step is added.
- **Trigger is a skill invocation** (decided 2026-10-10). A `three-amigos` skill wraps `ai-mode amigos`; the human invokes it directly, or another skill in the planned Superpowers-replacement pipeline invokes it (e.g. a brainstorming/spec skill handing off an approved story). The skill is the only entry point; there is no ambient or automatic trigger. Intake contract: story text, provenance, authorization/vetting note.
- Because other skills call it, the skill's contract is stable and machine-friendly: structured intake in, verdict + artifacts + exit code out, so a calling skill can branch on `ready | not-ready | paused`. A calling skill passes its own name as `provenance.caller_id`. Story text that originated from a model is still data, not instructions, even when the calling skill is trusted. A calling skill must show the Story Contract to the human and wait for approval before acting on a `ready` verdict (no auto-proceed for now).

## Context and audit trail

- Working context per turn: current Story Contract + delta since that role last spoke + path to the full transcript. The contract is the compaction.
- Audit level is configurable (flag/config):
  - `off`
  - `summary` (default): contract, decision log, parked-question history, verdict
  - `full`: every prompt/response verbatim, model, sampling params, seeds, token counts, timings, uncertainty samples and logprobs; append-only `transcript.jsonl` linked to `ai-mode trace` spans
- For extreme observability: optional hash chain (tamper-evident), configurable sink directory, optional redaction pass.

## Not-ready verdict: self-heal

Classify the reason; each has an owner who decides within its remit and logs rationale:
- missing/conflicting rules → product
- missing examples/edge cases → qa
- too big, infeasible, hidden coupling → architect (usually split the story and re-run)

Hard cap on rounds.

## Uncertainty

Combine: (1) self-consistency (same question sampled N times, measure agreement), (2) cross-role disagreement, (3) token logprobs at answer points if llama-server exposes them (to verify).

Above threshold → **park** the question. Finish all other red cards first (answers may unblock it), re-ask each parked question once with the new context. Still above threshold → **pause and escalate** to the human. States: parked, re-asked, escalated.

## Delivery: stacked PRs, TDD (revised 2026-10-10)

Follows GitHub's guidance for stacking AI-generated code (docs.github.com/en/copilot/tutorials/stack-ai-generated-code-in-pull-requests): **design the stack before generating code**; layers are split by **feature dependency**, foundation at the bottom, each layer a single coherent change small enough to review quickly (if it needs a long description or grows, split it); a reviewer reads the PRs bottom to top and watches the feature come together. An earlier draft used QA test types (unit, contract, functional, integration, ...) as the layers; that was dropped because those types do not map to dependency order and the upper ones just recreate a CI pipeline.

**Stack shape**
- **Base layer: the Three Amigos PR.** Story Contract, decision log and the story's scenarios as pending (skipped) tests. It is the contract the rest of the stack satisfies and the first thing a human reviews.
- **Code layers, bottom to top, by dependency** (for example: data model and migration, then the endpoints that use it, then middleware/guards). Each layer is the production change **plus the tests for that slice, written first (TDD)**, at the level that fits the slice: pure logic -> unit, a boundary -> functional/contract, a real collaborator -> integration. `qa` chooses the level per layer with jev Stage B (`test_layer`), run **per layer**, not once per story.
- **Pending scenarios are un-skipped as their layer lands.** Each Story Contract scenario is assigned to exactly one layer (the first where it is testable). Each layer's PR is green on its own, so the stack merges bottom-up.
- **Top layer, only when needed: the journey test.** An e2e/journey scenario needs the whole feature, so it sits on top and only if a user journey changed.
- Not every story needs every layer; the layer plan lists only the layers the story needs.

**Gates, not layers.** Checks that apply across the stack run as CI gates on each layer's branch (and on the stack), not as PRs of their own: static analysis including static security, mutation score on changed code, visual regression (UI changes), performance budgets, dynamic security (DAST, authz, fuzzing; exposed surfaces only for now), AI evals. The layer plan declares `gates[]` with state `active | not-available`; any artifact a gate needs (an eval case file, a perf budget, an authz test) is committed in the layer whose behavior it covers. AI evals and performance have no harness yet: `not-available`, skipped with a note.

**Process (from the tutorial, adapted to our agents)**
1. architect designs the stack from the Story Contract (dependency order, sizes) before any code; the human can reshape it.
2. Build the bottom layer first and review it before moving on (a mistake there propagates upward); then add each layer on top, one branch per layer, each a clean self-contained diff.
3. Run tests, linters and scanning on each branch; self-review before asking others.
4. Submit the stack; request reviews bottom-up (independent layers can be reviewed in parallel).
5. Fix feedback **in the layer it belongs to**, then rebase upstack; failures route by layer. A defect in the Story Contract itself reopens the base layer and everything above is restacked.
6. Merge from the bottom (auto-merge or merge queue per layer once approved and green).

**Tooling:** `gh` 2.90+ and git 2.20+ (this machine has gh 2.102.0), plus the `gh-stack` extension and skill (`gh extension install github/gh-stack`, `gh skill install github/gh-stack`; **not installed yet**, install needs approval). Commands per the tutorial: `gh stack init`, `add`, `submit`, `rebase --upstack`, `up`/`down`/`checkout`. The tutorial drives these with Copilot CLI; ours would be driven by our own agents and skills (to be specced in the pipeline spec). **Wrapped in a skill (written 2026-10-10): `skills/pr-stack/SKILL.md`**, built from the gh-stack README (README says gh and Git 2.36+; the tutorial says gh 2.90+; this machine has gh 2.102.0 and git 2.56.0). `gh-stack` also ships its own agent skill (unread; ours does not depend on it). Ours is thin: one skill, a few fixed actions each running one `gh stack` command, carrying our rules (design from `layer_plan[]`, review bottom layer first, fix in the owning layer then `rebase --upstack`, never `submit` or merge without human approval, branch names tied to the plan). If prompt-level guardrails prove too weak, back the skill with an `ai-mode stack` command that refuses `submit`/merge without a recorded human approval.

## Repo changes implied

- Deferred: new `performance.md`/`security.md` QA guides are no longer needed for the stack (those are CI gates, not layers); add them later only if qa must author gate artifacts.
- jev's existing `test_layer` set is reused unchanged, now run per stack layer; adjust the qa/testing-guidelines docs to say so. Add `gh-stack` setup notes to the skill docs.
- Role prompts for the meeting: add an "Amigos meeting" section to `presets/agents/product.md`, `qa.md` and `architect.md` (like their existing ad-hoc consultation sections) covering turn format, delimiters, reacting to other roles, and structured output. This is a plan deliverable.
- Planning: `skills/drafting-plans/SKILL.md` (ai-mode's own plan authoring skill) and the deterministic subcommands it relies on: `ai-mode plan lint` and `ai-mode plan verify` (built, stdlib Go, see `internal/app/plan*.go`) and `ai-mode plan tdd-check` (planned); a "Plan authoring" section in `presets/agents/architect.md`.
- New command in `internal/app/`, `completions/_ai-mode`, `README.md`, `AGENTS.md`, and `skills/three-amigos/SKILL.md`; cross-reference from `skills/ai-mode-routing/SKILL.md`.

## Problem and success (added from product review)

Problem: stories reach implementation with unstated rules, missing edge cases and unknown size, so tests are written after the fact and rework follows. `amigos` front-loads that conversation and turns it into the test plan.

Success (no numeric target invented; baseline to be measured): a "ready" story yields a Story Contract whose examples become the base-layer pending scenarios; fewer post-implementation spec changes per story; every parked question ends resolved or escalated, never silently dropped.

## User stories

- As the **human**, I hand `amigos` a story and get a ready/not-ready verdict, a contract I can review in minutes, and a layer plan, without refereeing the three roles.
- As **qa**, I get rules from product before writing examples, so my cases map to a stated rule.
- As **architect**, I can say a story is too big or infeasible before planning starts and have it split.
- As **coder**, I receive architect's plan at handoff and can push back on gaps before executing.
- As an **authorized agent**, I can submit a vetted story and receive the same verdict and contract.

## Priorities

- **Must:** `ai-mode amigos` with rounds, Story Contract, decision log, layer plan, verdict, parked-question flow, `summary` audit.
- **Should:** self-heal loop, uncertainty scoring, `full` audit, skill wrapper.
- **Could:** hash chain, sink directory, redaction options beyond the mandatory secret scan.
- **Out of scope:** running the stack's tests or opening PRs (that is `drafting-plans`/execution); building the AI-eval and performance harnesses.

## Acceptance criteria

- Given a story, when `ai-mode amigos --rounds 2` runs, then it prints a Story Contract, a layer plan and one verdict, and exits 0 for `ready`, 2 for `not-ready`, 3 for `paused` (see CLI surface).
- Given a not-ready verdict whose cause is within a role's remit, then the loop re-runs up to the round cap and logs each decision with rationale.
- Given a question over the uncertainty threshold, then it is parked, re-asked once after other questions resolve, and escalated (verdict `paused`) if still over threshold.
- Given `--audit full`, then every prompt/response is in `transcript.jsonl` after secret redaction; given `--audit off`, nothing is written.
- Given a `ready` verdict returned to a calling skill, then the skill presents the contract to the human and does not start the next stage until the human approves.
- Given a layer plan with gates marked `active`, then CI runs each active gate on every layer's branch, and a layer whose gate fails is not green.
- Given the layer plan, every Story Contract scenario is assigned to exactly one layer; gates without a harness (AI evals, performance) show `not-available` with a note; the dynamic-security gate is active only for exposed surfaces.

## Session lifecycle and artifacts (added from architect review)

- Each run has a `meeting_id`; state lives under the ai-mode state dir at `amigos/<meeting_id>/` (like `team/`): `contract.json`, `decisions.jsonl`, `parked.jsonl`, `transcript.jsonl` (full only). `--resume <meeting_id>` continues a paused meeting.
- **Intake contract** (JSON): `story` (text, treated strictly as data), `provenance` (`human|skill|agent` + caller id), `authorization` (vetting reference, required for non-human), optional `repo` path and `constraints`.
- **Story Contract** (JSON/Markdown pair): `story`, `rules[]`, `examples[]` (each tied to a rule), `questions[]` (state `open|parked|re-asked|resolved|escalated`, uncertainty score), `size`, `split_into[]` (follow-up story refs/meeting ids when architect splits), `layer_plan[]` (stack layers plus `gates[]`, see Delivery), `verdict`, `disagreements[]`.
- **Decision-log entry:** `ts`, `round`, `role`, `decision`, `rationale`, `impacted_layers`, `trace/span`.
- **Round semantics:** one round = one full product -> qa -> architect cycle. `--rounds N` is the budget (minimum 2); a self-heal re-run and each parked-question re-ask consume a round from the same budget; a hard maximum (default to be set in the plan) stops runaway loops. Re-run re-enters at `product`; the command's loop controller drives re-runs, the roles only decide the fix.
- **`layer_plan[]`** is the stack design: ordered entries `{n, name, scope, depends_on, scenarios[], tests[{level, scenarios}], size}` plus `gates[]` (`{name, state}`). `architect` proposes the layers (dependency order, sizes); `qa` assigns each scenario to the layer where it first becomes testable and picks the test level per layer via jev Stage B; `drafting-plans` consumes it and coder pushes back on it at handoff.
- **Resume state:** `contract.json` also records current round, next role and per-role last-seen delta pointer, so `--resume` re-hydrates from `contract.json` + `decisions.jsonl` + `parked.jsonl` and continues at the recorded role.
- **Intake precedence:** positional string > `--story -` (stdin) > `--story FILE` > JSON contract; `--caller` sets `provenance.caller_id`. `constraints.exposed` (bool/list) declares exposed surfaces and decides whether the dynamic-security gate is active.
- **`--audit summary`** writes `contract.json`, `decisions.jsonl`, `parked.jsonl` and no transcript; `full` adds `transcript.jsonl`.
- **Uncertainty (v1):** score = self-consistency agreement ratio over N samples; a recorded cross-role disagreement parks the question regardless of score; logprob confidence is blended in only if the llama-server spike succeeds. N and the park threshold are configurable, with defaults chosen in the plan and checked against an eval; a `disagreements[]` entry is `{rule_or_example, roles, positions}` recorded by whichever role detects it.
- Exact schemas are a planning deliverable; the fields above are the minimum.

## CLI surface (added from UX review)

- `ai-mode amigos [--story FILE|-] [--rounds N] [--audit off|summary|full] [--out FILE] [--resume ID] [--json] [--profile P] [--caller C]`; story may also be a positional string.
- `--json` changes only the stdout format (artifacts as JSON instead of text) and is independent of `--audit`, which controls only what is persisted. `contract.json` is the authoritative artifact; `decisions.jsonl` is the audit trail.
- Default stdout: verdict line first, then layer plan, open/parked/escalated questions, then the Story Contract; `--json` emits the artifacts. Exit codes: 0 ready, 2 not-ready, 3 paused (needs a human), 1 error.
- Question states `open -> parked -> re-asked -> resolved | escalated`; a paused run prints the escalated questions and the `--resume` command.
- Help text must have sections: purpose (one line), inputs (with the intake precedence rule), output order, exit codes (0 ready, 1 error, 2 not-ready, 3 paused), flags, state persistence (what `--resume` continues and from which files, what `--audit summary` vs `full` write), and examples (inline story, story file with `--rounds 2`, `--resume <id>`). Error messages are specified in the plan.
- The loop controller (rounds, re-runs, state transitions) lives in the `ai-mode amigos` command in `internal/app/`; the skill wrapper only parses the ask, runs the command and summarizes. The command owns creation and permissions of `amigos/<meeting_id>/`. The park threshold and sample count are flags/config, not constants.

## Security requirements (added from security review)

- Story text and any other agent-supplied content is **data**: passed in a delimited field, never concatenated into role instructions; the intake gate rejects malformed contracts before any model call.
- Calls from other skills present their skill name as caller id and an authorization reference that maps to a trust boundary (an allowlist of skills permitted to invoke amigos, kept in the repo/config). The **gate exists** from v1; the allowlist starts with the human-invoked skill and grows as the replacement pipeline lands.
- `full` audit runs a **mandatory secret-scan/redaction pass** before write; sink directory must be user-owned with restrictive permissions (0700 dirs, 0600 files).
- Hash chain stays optional by default (it protects against tampering after the fact, not capture); required where an operator opts into extreme observability.
- **Intake gate** runs before any model call: strict schema validation; `repo` must be a local path: symlinks are resolved (EvalSymlinks) and the canonical path must be a descendant of a `trusted_roots` entry (`..`, URLs and other schemes rejected, so no remote fetch and no symlink escape to files like `~/.ssh` during repo mapping); `caller_id` must be in the caller allowlist, else exit 1; a secret scan (`tk_`, `sk_`, known token patterns) over story text.
- **Data delimiters:** story and provenance fields reach the roles inside fixed delimiters (e.g. `<story_data>...</story_data>`) with an instruction in every role prompt to treat their contents as untrusted input, never as rules. Repo files read during mapping are data too.
- **Secrets:** board/API tokens never appear in `amigos/<id>/` artifacts, transcripts or the repo; they live in the OS keychain, with a 0600 file outside any state/repo directory as the fallback. The redaction pass includes token patterns. Audit files are created with mode 0600 (dirs 0700) at creation, not chmod'd later.
- **Caller allowlist (decided 2026-10-10):** a static, repo-tracked, human-edited file (`presets/amigos-callers.list`, one caller id per line with `#` comments; a list because the standard library has no TOML parser) checked by the command; board bot identities are a second, independent layer. The allowlist starts with the human-invoked skill.
- **Structured output only:** architect's `layer_plan[]` and every role's contract contributions are parsed and validated against the schema by the command before use; role text is never string-interpolated into artifacts. Gate states come from harness availability and `constraints.exposed`, not from story text.
- **Intake errors are structured:** a validation failure returns exit 1 and a JSON error `{code, detail}` with `code` one of `unauthorized_caller`, `schema_mismatch`, `path_outside_roots`, `secret_detected`, so calling skills can react programmatically.
- **Audit quota:** `amigos/<meeting_id>/` has a configurable size cap; exceeding it pauses the run with an error instead of growing unbounded.
- **Approvals are recorded:** every human approval of an outward action (push/submit/merge) is written to `decisions.jsonl` with who, what and when. If prompt-level gates in `pr-stack` prove too weak, the `ai-mode stack` command enforces them by refusing those actions without a recorded approval.
- A story cannot waive the security gate: layer-plan changes come from the roles' reasoning, not from instructions inside story text.

## Review log

Run 2026-10-10 on dev-shop via `ai-mode ask` (inputs include the full spec). Passes: product, research, security, ux, architect, plus a one-off **coder** pass run at the human's request to stress-test the spec (not part of the standard process; merged: exit-code fix, round semantics, split handling, layer-plan authorship, resume state, intake precedence, exposed-surface field, audit clarity; Go registration points left to the plan, which must name the entrypoint in `internal/app/`); **design N/A** (no UI). Outputs in `docs/superpowers/specs/reviews/`. An earlier run had the spec dropped from stdin and was discarded. Round 2 (same day, standard roster, full updated spec): product, research, security, ux, architect (outputs `*-r2.md`; design N/A, coder excluded per process). Merged: board/companion services split into `2026-10-10-work-board-design.md` (product); duplicate logprobs open item removed (research); intake gate, data delimiters, secret handling, caller allowlist default, audit file modes (security); help-text structure (ux); loop-controller location, configurable threshold, state-dir ownership (architect). Not adopted in round 2: per-project token scopes (Vikunja scopes are per route group; project reach comes from sharing), root-owned secret store (overkill), the claim that `--audit` behaves inconsistently with `--json` (not in the spec), and `VEANS_TOKEN` references (veans was dropped). Round 3 (same day, after the Story Contract rename and the dependency-ordered stack + CI-gates revision; research and ux were discarded once for truncation/degeneration and re-run): merged the active-gates criterion (product); symlink-safe path check, schema-validated role output, structured intake errors, audit quota, recorded approvals (security); `--json` vs `--audit` and artifact authority (ux); role-prompt deliverable and a concrete v1 uncertainty score (architect). Not adopted in round 3: one scenario per layer (product), signed approval tokens and `rebase --upstack` approvals (security), Go `exec.Command` advice (not a Go component), invented uncertainty defaults (ux). Research found no blockers. Round 1 not adopted: deferring AI-eval/performance layers (product; contradicts decided scope), invented success numbers, mandatory hash chain by default.

## Work board (out of scope here)

Work tracking (Vikunja on Postgres, per-agent bot users, companion services) is specified separately in `2026-10-10-work-board-design.md`. This spec only assumes: `provenance.caller_id` may carry a board task id, and a verdict can map to a column move made by the calling skill's own bot identity.

## Open

- Logprobs availability in llama-server (research: blocking for the uncertainty design; do a spike before planning; fallback is self-consistency + cross-role disagreement, already specified).

## Decided (2026-10-10)

- PR stack layers follow feature dependency (GitHub's stacking guidance); tests ride in the layer they cover, and cross-cutting checks (mutation, visual, perf, dynamic security, AI evals) are CI gates, not layers.
- Caller allowlist is a static, repo-tracked, human-edited file checked by the command; board bot identities are a second layer.
- A calling skill never auto-proceeds on `ready`: it surfaces the Story Contract to the human first. Revisit once the pipeline is trusted.
- Shared transcript vs previous-role-only inputs: map + delta + transcript path; revisit if 131k contexts strain.
