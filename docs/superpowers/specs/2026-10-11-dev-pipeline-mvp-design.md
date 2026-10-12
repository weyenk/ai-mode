# Dev pipeline MVP: run one request through all ten stages

**Status:** working design, 2026-10-11. Written for an unattended build, so the decisions the roadmap leaves open are
made here and marked **Decided here**; each is cheap to revisit.
**Roadmap:** `docs/superpowers/plans/2026-10-11-dev-pipeline-roadmap.md` (stage order, contracts, invariants, D1–D4).
**Acceptance run:** a "simple pomodoro timer" feature request goes from intake to open draft PRs with no human step
other than intake, and the code in those PRs was written by the local roles, not by the operator. The pomodoro module was
only a test target for the workflow; its code (draft PRs weyenk/ai-mode#19 and #20) and its `docs/pipeline/` records are
not kept on this branch (they remain in its git history).

## What this changes in the roadmap

The roadmap builds the pipeline in the order P5–P7, P8–P9, P1–P4, because the work-board spec enters at stage 5. A new
request enters at stage 1, so this MVP builds a thin version of **every** stage first and runs the request through each
as it lands. Each thin stage keeps its roadmap contract (inputs, outputs, exit gate, roles, card note) so a later
sub-plan can deepen it without changing its neighbours.

| Roadmap item | MVP version | Left for the full sub-plan |
| --- | --- | --- |
| P1 work board | File board: one JSON file per card under `docs/pipeline/board/`, rendered to `BOARD.md`; same columns, `stage:`/`type:` labels, parent links and card notes. Since 2026-10-12 mirrored both ways onto Vikunja (see "Work board" below) | `init` card templates; human moves other than intake and done |
| P2 scheduler | `ai-mode pipeline tick` (lock, one stage on one card, commit, optional push) and `ai-mode pipeline run` (ticks until nothing can advance); `ai-mode pipeline launchd` prints a LaunchAgent plist but does not install it | installing the job; starting dev-shop from launchd |
| P3 viability | `viability` stage on `ai-mode team` roles with per-role JSON estimates; ROI and go/no-go computed by code from a context rubric | richer rubrics, research with browsing |
| P4 spec | architect decides the split, writes each spec as an agent; specialist review passes (product → research → security → ux → design → architect) as consults; chief merges as an agent that may write only the spec | user-flow authoring by ux as its own step |
| P5 feature plan | `ai-mode plan lint --feature`; architect writes the feature plan as an agent and must get the lint clean | sizing rules beyond the lint |
| P6 stories | story files under `docs/pipeline/stories/`, worded by product, in the intake format `ai-mode amigos --story` accepts | board cards beyond the file board |
| P7 amigos | ready needs all three amigos; no human approval; `pipeline` caller; verdict saved next to the story | — |
| P8a kickoff | `ai-mode work start <story>` refuses non-ready stories, has architect write the story plan, then runs qa (red) and coder (green) per task in a dedicated worktree | parallel tasks |
| P8b TDD gate | `ai-mode plan tdd-check`: scenario test names exist, red commit fails, tip passes, tests unchanged after red | mutation checks |
| P8c PR stack | layers become a linear `gh stack`; stage 10 pushes and opens draft PRs (D3); `gh pr merge` and `gh stack merge` blocked for every model | — |
| P9 review | architect reviews each layer's diff against the Story Contract (JSON comments); coder fixes in the owning layer, `rebase --upstack`, round cap 3 then `needs-human` | security reviewer on exposed layers |
| P-flags | feature-plan lint and Story Contract carry flags; no runtime mechanism | the flag mechanism (needed before the first flagged story reaches stage 8) |

## Decisions

- **Decided here — agent runtime (open in P2/P8a): an `ai-mode agent` loop, not Qwen Code headless.** llama-server
  returns OpenAI tool calls for coder (Qwen3.8) and architect (Gemma 4) with `jinja = 1` (checked 2026-10-11). The loop gives
  a role a fixed tool set: `list_files`, `read_file`, `search`, `write_file`/`edit_file` (only paths the task owns),
  `run` (an allowlist: `go test|build|vet|run`, `gofmt`, `ai-mode plan lint|verify`; argv only, no shell), and `finish`. That
  enforces exclusive ownership and "only a human merges" in code instead of in a prompt, needs no `--yolo`, and is testable
  with a fake model. Each run writes a transcript (`agent/<run-id>.jsonl` in the state dir) and an `agent` event.
- **Decided here — board until P1 lands:** a file board in the target repo, committed with the work, so the history is in
  git and readable on GitHub. Card notes use the roadmap's five fields (Stage, Done, Artifacts, Why, Next) and the bot
  user names from the roadmap as authors.
- **Decided here — layout.** A pipeline runs against a **target**: a Go module directory (`--repo DIR`) inside a git
  repo. Its artifacts live under `DIR/docs/pipeline/` (`board/`, `requests/`, `viability/`, `specs/`, `plans/`,
  `stories/`, `reviews/`). Implementation happens in a separate git worktree per feature plan (state dir
  `pipeline/worktrees/<plan-card>`), so the pipeline branch keeps the artifacts and the stack branches keep only code.
- **Decided here — one linear stack per feature plan.** Stories run in dependency order; each adds its layers on top of
  the previous story's, starting from the pipeline branch. A reviewer reads bottom-up across stories.
- **Decided here — stage labels.** Columns and labels follow the roadmap. Two labels are added inside `building`:
  `handoff` (review passed, PRs not yet open) and parents waiting on children carry `tracking` and take the least
  advanced column of their children (that is the roadmap's "a parent moves to review when all its stories are there").
- **Decided here — who answers a paused amigos question in the acceptance run:** the operator, acting as the requester,
  and only for product questions about the request it wrote; the answer is logged on the card as the requester's. A
  question about the pipeline itself or anything risky goes to the human.

## Stage table (MVP)

| # | Runs on | Stage label in → out | Roles (mode) | Artifact | Note by |
| --- | --- | --- | --- | --- | --- |
| 1 | — | → `new` (`init`) | human (`pipeline new`) | `requests/<card>.md` | human |
| 3 | request | `new` → `spec` (`ideation`), or `needs-human` on no-go | research, product, architect, + security for health/enterprise (consult) | `viability/<card>.md` | bot-product |
| 4 | request | `spec` → `tracking`; creates spec cards at `spec` | architect (consult: split) | spec cards | bot-architect |
| 4 | spec | `spec` → `planning` | architect (agent: write), six reviewers (consult), chief (agent: merge, spec file only) | `specs/<card>.md`, `specs/reviews/<card>-<role>.md` | bot-architect |
| 5 | spec | `planning` → `tracking`; creates plan card at `planning` | architect (agent) + `plan lint --feature` | `plans/<card>.md` | bot-architect |
| 6 | plan | `planning` → `tracking`; creates story cards at `amigos` | product (consult: wording) | `stories/<card>.md` | bot-architect |
| 7 | story | `amigos` → `ready`, or `needs-human` | product, qa, architect (`ai-mode amigos`) | `stories/<card>.contract.json` | bot-amigos |
| 8 | story | `ready` → `in-progress` → `architect-review` | architect (agent: story plan), qa (agent: red), coder (agent: green) | `stories/<card>.plan.md`, commits on the stack | bot-qa, bot-coder |
| 9 | story | `architect-review` → `handoff`, or `needs-human` after 3 rounds | architect (consult), coder (agent: fixes) | `reviews/<card>/layer-<n>-round-<r>.json` | bot-architect |
| 10 | story | `handoff` → `review` | docs (consult: PR text), `gh stack push`, `gh stack submit --auto` (drafts) | PR links on the card | bot-chief |

A tick picks the card to advance in this order: a card already in flight (deepest first, stories in dependency order,
a story waits until the stories it depends on are at `review`), otherwise the oldest `new` request. One feature in flight
at a time (roadmap invariant).

## Invariants this MVP enforces in code

- No merge: `pipeline` never calls merge; the agent `run` allowlist has no `git` or `gh`; the chief guard's shell denial
  blocks `gh pr merge` and `gh stack merge` for every model (P8c).
- Chief writes no code: chief runs only as a consult or as a spec-merge agent whose only writable path is the spec.
- Coder starts only on a ready story: `work start` reads the saved verdict and refuses anything but `ready`.
- Every stage notes its card; a stage that cannot finish moves the card to `blocked`/`needs-human` with a note saying
  what the human must decide, and stops.
- Model output is data: stage inputs come from artifacts, never from notes; role replies are parsed into fixed shapes
  (JSON estimates, review comments) or written to files the next stage reads as context.

## As built: what the first run changed (2026-10-11)

Running the pomodoro request through the real dev-shop roles exposed failures that unit tests could not; each was fixed
in the tool, not by hand-editing an artifact:

- **Story plans are JSON, rendered to markdown.** In a stage 8 dry run architect spent fifteen `plan lint` rounds on the
  drafting-plans markdown's format (backtick-quoted excerpts, a truncated packet, edits that no longer matched) and split
  tests from implementation. It now writes `<story>.plan.json`; `ai-mode plan render` writes the drafting-plans document
  (format right by construction) and enforces the pipeline's rules (a task covering scenarios owns both its test and its
  code file; every contract scenario covered). The same dry run then planned in seven turns, first time clean.
- **amigos agreement is judged by decision, not wording.** Self-consistency compared sampled answers by exact string, so
  three answers that all said "exit code 0" scored 0.33 and settled questions were parked and escalated to the human. The
  `fast` model now groups answers by decision (exact match is the fallback), and a sample whose answer rewords the
  question still counts.
- **amigos examples are de-duplicated by text and the layer plan's bookkeeping is repaired.** A meeting where all three
  agreed ended not-ready because roles re-sent examples under reworded rules (27 examples) and architect's layer plan
  listed only the first six. Leftover examples now join the layer holding their rule; each repair is logged.
- **amigos turns know their round.** qa kept adding rules and questions every round while product and architect agreed;
  the last round now says the story is ready only if all three set ready, and polish becomes a follow-up story.
- **The stack is built on a recorded base commit.** The pipeline commits notes to its branch while stage 8 runs; building
  layer 1 on the moving branch collided with files the worktree held untracked.
- **Agent `run` treats quoted shell characters as argv** (`go test -run '^(A|B)$'` was refused).
- **Branch builds need `AI_MODE_PRESETS`** pointed at the branch's `presets/` (the user config pins the main checkout).
- **Requester answers in the acceptance run** were given by the operator from the original request and are logged on the
  cards as such (design decision above).

### Later in the run (stages 8-10)

- **Contract defects are ruled on, not coded around.** Amigos examples contradicted each other (a 25-minute focus ending at
  t=1440s in one example and t=1500s in another; a status line format the requester had answered differently). qa
  encodes the contract faithfully, so: architect reviews qa's tests before red is committed, triages a green phase coder
  cannot reach (a wrong test is fixed by qa as a `Phase: review` commit), and every ruling is appended to the Story
  Contract under **Amendments**.
- **Thinking is capped per request.** qa and coder (Qwen3.8 at ~24 tok/s) spent single turns of 16-26k thinking tokens
  (12-19 minutes); the pipeline now sends `reasoning_effort: low` and `thinking_budget_tokens: 8192` with each of their
  agent turns (llama-server honours both; no restart), and story plans keep tasks to at most 6 scenarios.
- **Re-planning a started story keeps its committed tasks;** code packets may only create new files (the JSON plan has no
  edit modes, and a whole-file rewrite dropped a type the module used).
- **A program must be runnable, and stage 9 checks it like a user.** Every part was built and tested while `main` stayed an
  empty stub, then a wired `main` that validated flags and exited. The feature's last story must now wire `main` (a task
  owning that file, with a journey test), and stage 9 adds a journey review: it builds the program, runs it for 3 seconds,
  sends Ctrl-C, and gives architect the output plus what a terminal shows (`\r`, erase-to-end and bells applied) before
  and after Ctrl-C. A checklist "yes" must quote text the user can see; fixes go to the layer that owns the file, test-first
  (qa, then coder), with the layers above rebased. This caught a Ctrl-C handler that never printed its summary and a
  status line that erased itself as soon as it was drawn.
- **The human's review column works as designed:** the operator ran the program from the review column, found it did
  nothing, and reopened the story (`pipeline reopen`); notes from humans are recorded but never become stage inputs.

Result of the acceptance run: request C0001 → viability GO → 1 spec (6 reviews) → feature plan with 2 stories → amigos
(two requester clarifications) → 10 tasks built test-first on a 2-layer stack → architect and journey review → draft PRs
weyenk/ai-mode#19 and weyenk/ai-mode#20. The program counts down in place, rings and switches to a break at the end of a
session, prints `Completed N focus sessions` on Ctrl-C, and rejects lengths outside 1-1440 minutes with exit status 2.

## Work board (2026-10-12)

The MVP first shipped with the file board alone, which was not the roadmap's board. The Vikunja board is now in place:

- **Vikunja runs outside ai-mode, 24/7.** Vikunja v2.7.0 (official macOS arm64 release, sha256 checked) under a user
  LaunchAgent `local.vikunja` (RunAtLoad, KeepAlive), on Homebrew Postgres 17 (`brew services`), both on loopback only.
  Postgres instead of SQLite because the 2026-10-10 trial lost most concurrent writes on SQLite and none on Postgres. A
  vector database cannot back Vikunja (it supports SQLite, MySQL and Postgres); if agents later want semantic search over
  cards, pgvector in the same Postgres covers it. LaunchAgents run while the owner is logged in; a LaunchDaemon (root)
  would be needed to run before login.
- **ai-mode is a client.** `internal/app/vikunja.go` is a thin stdlib client for `/api/v2`, checked against the server's
  OpenAPI spec. `ai-mode board init` is run by the human (password prompt in the terminal, never stored): it creates one
  bot user per agent (`bot-chief`, `bot-product`, `bot-architect`, `bot-amigos`, `bot-qa`, `bot-coder`) and stores a
  scoped token for each, plus one for ai-mode on the owner's account, in `~/.config/ai-mode/board.json` (0600). Bot tokens
  cannot delete anything or create users or tokens. `ai-mode doctor` reports the board unreachable or the tokens refused.
- **The repo's card files stay the record; the board mirrors them and takes two kinds of human input.** Push (in
  `pipeline.sync`, after every stage): one task per card in its column's bucket, labels, `parenttask` and `follows`
  relations, and each card note as a comment by the bot that wrote it. Pull (start of every tick): a task a human adds to
  `init` becomes a request card, and a card a human moves to `done` becomes done. Vikunja does not record who moves a task,
  so that note says "human (on the board)". Other edits on the board are overwritten; corrections go through `pipeline
  retry|reopen|note|answer`. The cards carry `board_task` and `board_notes` so a sync never duplicates a task or a comment.
