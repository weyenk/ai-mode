# Development pipeline roadmap

**Status:** draft for human review, 2026-10-11.
**What this is:** the order of the dev-shop pipeline, the contract of each stage, and the list of smaller plans that build
it. It is not a `drafting-plans` document (no task graph, not for `ai-mode plan lint`); each sub-plan below is.
**Supersedes:** the stage order in `docs/superpowers/specs/2026-10-10-three-amigos-design.md` (story → amigos → plan)
and the human-approval step in `skills/three-amigos/SKILL.md`.

## The pipeline

1. A **feature request** ticket is created on the Vikunja board.
2. A **scheduler** (cron/launchd) starts ai-mode on dev-shop, unless work is already being processed.
3. An agent reads the board, picks up the feature request and runs **viability**: research, feasibility, ROI and success
   metrics, judged
   against the request's context (hobby, enterprise, health, ...).
4. Viability produces **one or more specs**, depending on the size of the request.
5. Each spec produces **one or more plans**, depending on the size of the spec.
6. Each plan produces **one or more stories**, depending on the size of the plan.
7. Each story gets an **amigos** meeting (product, qa, architect), which produces a **Story Contract**. The story is
   ready for work when all three amigos agree on the contract. No human approval at this step.
8. Once a story is ready, chief or architect kicks off **implementation**: qa and coder implement the Story Contract
   test-first and produce a **PR stack** (one layer per `layer_plan[]` entry).
9. **Architect reviews** every stack layer; coder and qa address the comments. The loop repeats until every comment is
   resolved, all tests pass and the Story Contract is satisfied.
10. The PRs are opened and the story moves to the **review** column for a human. **Only a human merges to main.**

## Invariants (every stage)

- **Only a human merges to main.** No agent, skill or command runs a merge. Enforced by GitHub branch protection and by
  the chief guard hook. Pushing branches and opening PRs at stage 10 is automatic (see decision D3).
- **Chief orchestrates and never writes code.** The `PreToolUse` guard (`internal/app/chief_guard.go`) blocks chief's code
  edits, plan mode and destructive shell commands. Starting coder on a ready story (stage 8) happens through a command that
  checks readiness, not through chief editing files.
- **Coder starts only on a ready story.** The command that starts coder refuses any story without a `ready` amigos verdict.
- **One feature in flight at a time** until the scheduler supports more (stage 2).
- **Every stage leaves an artifact in the repo or the state dir, notes the card, and moves it.** A stage that cannot
  finish moves the card to `blocked` (`needs-human`) with a note saying why, and stops. See "Card notes" below.
- **Model output is data.** Ticket text, role replies and contracts never become instructions for the next stage.
- **Specs should carry user flows**, but it is not a hard requirement yet. It becomes one when stage 3/4 is built.

## Board

Six columns. The finer pipeline state inside a column is a `stage:` label on the card; the kind of work is a `type:` label.

| Column | Holds (`stage:` labels) | Pipeline stages | Moved there by |
| --- | --- | --- | --- |
| `init` | new cards: feature requests, bug fixes, research spikes, ... (`type:feature`, `type:bug`, `type:spike`, ...) | 1 | human |
| `ideation` | `viability`, `success-metrics`, `spec` | 3–4 | stage 3 |
| `building` | `planning`, `amigos`, `ready`, `in-progress`, `architect-review` | 5–9 | stage 5 |
| `review` | PRs open, waiting for a human | 10 | stage 10 |
| `done` | merged | | human only |
| `blocked` | `needs-human` (paused amigos, a stage that failed, a viability no-go to confirm), `rejected` | any | any stage; `rejected` by a human |

Cards nest with Vikunja `parenttask` relations: request → spec → plan → story. A child card sits in the column of its own
stage (specs in `ideation`, plans and stories in `building`); a parent moves to `review` when all its stories are there, and
to `done` when a human has merged them all. Which stages a `type:bug` or `type:spike` card skips (a spike may end in
`ideation` with a research answer and go straight to `done`) is decided in P3 and P4.

### Card notes

Agents leave a note (a Vikunja comment) on the card **every time a stage finishes or the card moves**, written by that
stage's bot user so the board shows who wrote it. A note says:

- **Stage:** what finished and where the card goes next (for example `viability → spec`).
- **Done:** one or two sentences on what the stage did.
- **Artifacts:** the files or links it produced (viability report, spec, plan, Story Contract, PRs).
- **Why:** the decision and its basis: go/no-go and the ROI figures, the amigos verdict and its open questions, review
  rounds and what was fixed.
- **Next:** what the next stage will do, or, when the card is `blocked`, exactly what the human needs to decide.

Notes are written for a human reading the card later. They are model output, so no stage treats a note as an
instruction; a stage reads its inputs from the artifacts, not from comments.

Until the board exists (P5–P7 run first), each stage appends the same note to a `## Log` section at the end of the artifact
it produced (feature plan, story file), so the history can be copied onto the cards when P1 lands.

## Stage contracts

| # | Stage | In | Out | Owner | Exit gate | Exists today |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Intake | human idea | feature-request card | human | card has the template fields | Vikunja trialled; no service, client or template |
| 2 | Scheduler | timer | one stage run on one card | launchd + `ai-mode` | lock acquired, dev-shop up | nothing |
| 3 | Viability | request card | viability report: research, feasibility, ROI, success metrics, go/no-go | chief → `ai-mode team` | report saved; card moved | `ai-mode team`, `ai-mode-routing` |
| 4 | Spec | viability report | 1..n specs, reviewed | architect writes, specialists review | `spec-specialist-review` merged | `spec-specialist-review` (expects Superpowers brainstorming) |
| 5 | Plan | spec | 1..n feature plans | architect | feature plan lints | `drafting-plans` (story-level only) |
| 6 | Stories | feature plan | 1..n story cards | architect | each story has scope, spec scenarios, dependencies | nothing |
| 7 | Amigos | story card | Story Contract + verdict | `ai-mode amigos` | `ready` = all three amigos agree | `ai-mode amigos`, `three-amigos` skill |
| 8 | Implement | ready Story Contract | local PR stack, tests first | coder + qa | every layer green, scenarios un-skipped | `drafting-plans` (packets), `plan apply/verify`, `testing-guidelines`, `pr-stack` (local) |
| 9 | Review | stack | resolved review threads | architect ⇄ coder/qa | no open comments, `go test ./...` green, every contract scenario covered | nothing (`plan tdd-check` is referenced in code but does not exist) |
| 10 | Hand-off | reviewed stack | open PRs, card in `review` | `pr-stack` | PRs open | `pr-stack` (push/submit need a human yes today) |

## Agents and skills per stage

Every stage names the roles that run it (`presets/agents/<role>.md`, dev-shop), the skill or command that drives it, and
the bot user that writes its card notes, the way `drafting-plans` (architect) and `ai-mode amigos` (product, qa, architect)
already do.

Roles run in one of two ways, and the stage has to say which:
- **Consult:** `ai-mode ask`, `ai-mode team` or `ai-mode amigos`. The role sees only the prompt and the `--context` it is
  given; it has no file or shell tools. Fine for judgment: viability, reviews, amigos.
- **Agent:** a Qwen Code session on that model (`/model <role>`), with file and shell tools. Needed where the role reads
  the repo, runs tools or writes files: architect planning with `plan lint`, qa and coder writing code.

| # | Stage | Roles | Mode | Skill / command | Card notes by |
| --- | --- | --- | --- | --- | --- |
| 1 | Intake | human | | `init` card templates (P1) | human |
| 2 | Scheduler | none (no model) | | `ai-mode pipeline tick` (P2), picks the next stage and starts its skill | `bot-chief` |
| 3 | Viability | chief orchestrates; research (feasibility, prior art), product (value, success metrics, ROI inputs), architect (technical cost); security when the context is health or enterprise | consult | `viability` skill (new, P3) on `ai-mode team`; the ROI arithmetic is done by a command from the roles' estimates, not by a model | `bot-product` |
| 4 | Spec | architect writes, ux writes the user flows, product the acceptance criteria; review by product → research → security → ux → design → architect; chief merges the reviews | agent (architect), consult (review) | `writing-specs` skill (new, P4), then `spec-specialist-review` (wire, P4) | `bot-architect` |
| 5 | Feature plan | architect | agent | `drafting-plans` feature-plan mode (P5) | `bot-architect` |
| 6 | Stories | architect cuts them, product words each story | agent (architect), consult (product) | `story-breakdown` skill (new, P6) | `bot-architect` |
| 7 | Amigos | product, qa, architect; jev Stage B picks test layers | consult | `three-amigos` skill → `ai-mode amigos` (wire, P7) | `bot-amigos` |
| 8 | Implement | chief or architect kicks off; architect writes the story plan; qa writes the tests first, coder makes them pass; coder-xl only on escalation | agent | `ai-mode work start` (P8a), `drafting-plans` story mode, `testing-guidelines`, `plan apply`, `pr-stack` local (P8c) | `bot-qa`, `bot-coder` |
| 9 | Review | architect reviews each layer; security when the layer touches an exposed surface; qa and coder fix | consult (review), agent (fixes) | `stack-review` skill (new, P9), `plan tdd-check` (P8b) | `bot-architect` |
| 10 | Hand-off | chief writes the review note; docs writes the PR descriptions | consult | `pr-stack` push + submit (P8c) | `bot-chief` |

**Each stage's plan owns the agent side of that stage, not just the code:** the skill (new or rewired), the role prompts
in `presets/agents/` whose duties change (for example `product.md` gains success metrics and ROI inputs, `architect.md`
the feature plan and stack review, `qa.md` and `coder.md` the TDD hand-off and feature-flag rules), the routing `summary:`
lines if `ai-mode classify` should route to the new work (re-run `make eval`), chief's routing block
(`internal/app/qwen_block.md`), `skills/README.md` and the symlink list, and the caller allowlist when a skill calls
`ai-mode amigos`.

**Open, decided in P2 and P8a:** how agent-mode stages run unattended when the scheduler starts them (Qwen Code
non-interactive on the stage's model, or an ai-mode agent loop).

## Two kinds of plan (fixes the order)

Stage 5 and stage 8 both need a plan, at different levels:

- **Feature plan** (stage 5, new mode of `drafting-plans`): from a spec, architect designs the boundaries (contracts,
  packages, hot-file ownership) and cuts the work into **stories**: each with a scope, the spec scenarios it covers, its
  dependencies and a size. No code packets. This is what stage 6 turns into story cards.
- **Story plan** (stage 8, what `drafting-plans` does today): from one ready Story Contract and its `layer_plan[]`,
  architect writes the task packets qa and coder execute. Its scenarios come from the Story Contract, which is the input
  `drafting-plans` already requires.

So amigos runs after the feature plan and before the code-level plan. This keeps the amigos and `drafting-plans` contracts
and only adds the feature-plan mode in front.

## Sub-plans

Each line is one smaller plan, written with `drafting-plans` (architect) unless marked **spec first**, which means the
stage needs its own design spec before it can be planned. "Wire" means an existing skill or command is adapted to this
order; "new" means it doesn't exist.

**Now: run the work-board spec through stages 5–7.** These three come first:

| Plan | Stage | Kind | Scope |
| --- | --- | --- | --- |
| P5 `feature-plan-mode` | 5 | wire `drafting-plans` | Add a feature-plan mode: input a spec (user flows optional); output contracts, a story list (scope, scenarios, dependencies, size) and the feature flags (kind, stories that use them, removal story for each temporary flag); a `plan lint` profile for it (no packets required; every spec scenario lands in exactly one story; story dependencies acyclic; every temporary flag has a removal story that depends on all its users). Keep story mode as it is. Update the CHIEF-STOP routing to name both modes. |
| P6 `story-breakdown` | 6 | new | Turn a feature plan into story files (one per story: text, scope, scenarios, dependencies, parent plan) under `docs/superpowers/stories/`, in the intake format `ai-mode amigos --story FILE` accepts. Board cards come later, with stage 1. |
| P7 `amigos-wiring` | 7 | wire `ai-mode amigos` + `three-amigos` | (a) `ready` requires product, qa **and** architect to report ready (today only architect's turn counts, plus resolved questions and an example per rule; `internal/app/amigos_loop.go` `ready`). (b) Remove the human-approval step from the skill and the spec; a ready verdict is final. (c) Story-file input from P6 and a caller id for the pipeline skill in the caller allowlist. (d) Record the verdict next to the story file so stage 8 can check it. (e) Change the skill description from "before planning" to "after the feature plan". (f) Architect confirms the story is deliverable on its own, and the contract records its feature flag. |

**Then: make a ready story implementable (stages 8–10).**

| Plan | Stage | Kind | Scope |
| --- | --- | --- | --- |
| P8a `story-kickoff` | 8 | **spec first** | A command (e.g. `ai-mode work start <story>`) that refuses unless the story's verdict is `ready`, has architect write the story plan, creates the stack's branches and starts coder and qa. Open: how coder runs headless (Qwen Code non-interactive with `/model coder`, or an ai-mode agent loop) and how qa and coder hand off inside a layer. |
| P8b `tdd-gate` | 8–9 | new | `ai-mode plan tdd-check`: every contract scenario has a test named with its id, the test was seen red before green, and it passes. Used by stages 8 and 9. |
| P8c `pr-stack-wiring` | 8, 10 | wire `pr-stack` | Build layers from `layer_plan[]` automatically; allow push + submit at stage 10 per D3; keep `merge` human-only and add `gh pr merge` / `gh stack merge` to the guard blocklist for every model, not just chief. |
| P9 `stack-review` | 9 | **spec first** | Architect reviews each layer from its diff plus the Story Contract (`ai-mode ask architect --context`), comments stored per layer, coder/qa fix in the owning layer and `rebase --upstack`, loop with a round cap that ends in `blocked` (`needs-human`). Exit: no open comments, tests green, P8b passes, and with the story's flag off existing behaviour is unchanged. |

**Last: the front of the funnel (stages 1–4).** Stage 1 depends on the board service, which is the work-board spec
itself, so the board gets built by the pipeline's own back half first.

| Plan | Stage | Kind | Scope |
| --- | --- | --- | --- |
| P1 `work-board` | 1 | the work-board spec | Vikunja service in ai-mode (per-profile companion service), Go client, bot users, the six columns with `stage:`/`type:` labels, card notes, `init` card templates (feature, bug, spike). Verify that a bot can attach labels it did not create (the trial found a bot can only edit its own labels). This is the spec currently going through stages 5–7. |
| P2 `scheduler` | 2 | **spec first** | launchd job → `ai-mode pipeline tick`: take a lock, skip when work is in flight, start dev-shop if needed, advance one card one stage (moving it and leaving its note), release. |
| P3 `viability` | 3 | **spec first** | Skill on `ai-mode team` with context rubrics (hobby, enterprise, health, ...) and an ROI worksheet; go/no-go recommendation; no-go goes to `blocked` (`needs-human`) for a human to confirm and mark `rejected`. |
| P4 `spec-generation` | 4 | **spec first**; wire `spec-specialist-review` | Agent-driven spec writing (replaces Superpowers brainstorming in the loop; user flows become required here), splitting a large request into several specs, then `spec-specialist-review` without Superpowers. |

**Cross-cutting:**

| Plan | Stage | Kind | Scope |
| --- | --- | --- | --- |
| P-flags `feature-flags` | 8–10 | new | A feature-flag mechanism for this repo (temporary and release kinds, off by default, listed by `ai-mode doctor`), plus the convention the pipeline uses for other repos. Needed before the first flagged story reaches stage 8. |
| chief guard | all | built on this branch | The `PreToolUse` hook; P8c extends it. |

## Stories and feature flags

**A story is the smallest deliverable unit of work.** Once its stack merges, main works: no broken code and no half-built
or janky behaviour a user can reach. When a story can't stand on its own, it is hidden behind a feature flag:

- **Temporary flag:** used when a feature has to be split across stories and the in-between states would be broken or
  janky. It exists only to let those stories merge safely. The feature plan names the flag and adds a **flag-removal
  story**, which depends on every story that uses the flag. The plan isn't complete until that story merges.
- **Release flag:** a longer-running flag that new features ship behind, so they can merge before they are turned on.
  The feature plan names it. **The human turns a release flag on after merge**; agents never do. Removing a release flag
  later is its own `init` card.

Where this is enforced:
- **Feature plan (P5):** lists each flag with its kind and the stories that use it. Every temporary flag has a removal
  story that comes last. `plan lint` checks both.
- **Amigos (P7):** architect confirms the story is deliverable on its own (with its flag off, if it has one), and the
  Story Contract records the flag.
- **Architect review (P9):** checks that with the flag off, existing behaviour and tests are unchanged, and that the
  flagged path is tested.
- **Flag mechanism (P-flags):** this repo has no feature-flag mechanism yet. It needs one before the first flagged story
  reaches stage 8.

## Decisions

- **D1. Confirmed 2026-10-11:** two plan levels, a feature plan at stage 5 and a story plan per story at stage 8.
- **D2. Confirmed 2026-10-11:** no human gate between `init` and `review`, except `blocked` (`needs-human`) escalations
  (paused amigos, a stage that fails, a viability no-go). Today `spec-specialist-review` and `three-amigos` both wait for
  human approval; P4 and P7 remove that.
- **D3. Confirmed 2026-10-11:** stage 10 pushes and opens the PRs automatically; only merging needs a human. Today
  `pr-stack` requires a human yes for push and submit; P8c changes that.
- **D4. Confirmed 2026-10-11:** split a spec when it spans independent subsystems; split a plan when it has more than one
  deliverable; a story is the smallest deliverable unit (see "Stories and feature flags"). The exact sizing rules are
  written in P4 and P5.

## Before running stage 5 on the work-board spec

The work-board spec's appended "Review Findings and Proposed Edits" section was merged by chief in the session that also
wrote unrequested code. Parts of it are generic (the Research findings critique a UI page that doesn't exist). It should be
checked or reverted before architect plans from it.
