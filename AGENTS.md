# AGENTS.md

Guidance for anyone (human or LLM) working **in this repo**. For end-user usage,
see [`README.md`](README.md). Model **catalog**: [`presets/MODELS.md`](presets/MODELS.md). Rationale / memory notes: [`presets/.local/MODELS-notes.md`](presets/.local/MODELS-notes.md) (local, gitignored).

## Purpose

`ai-mode` is a system-wide CLI that switches **llama-server** profile presets and
loads per-role **agent** system prompts. A profile is one `llama-server` process
serving many model "roles" (chief, jev, coder, …) on one port. You pick a studio
(`dev-shop`, `learning-center`, `av-club`), and ai-mode starts the router, prefetches
orchestration weights, and exposes an OpenAI-compatible API.

Everything runs **locally** on a Mac Studio M5 Max / 128 GB. No cloud models.

## Layout

| Path | What |
| --- | --- |
| `cmd/ai-mode`, `internal/app/` | The CLI (Go, stdlib only; `make build` → `dist/ai-mode`). Source of truth for commands/flags. |
| `presets/*.ini` | llama-server model presets. `[section]` = a role; `[*]` = shared defaults. |
| `presets/*.mode` | Sidecar metadata: `port`, `models-max`, `host`, `description`, `warm`. |
| `presets/agents/*.md` | System prompt per role. Frontmatter `role:` must match an ini `[section]`. |
| `presets/MODELS.md` | Per-profile model catalog (ini + mode). Keep in sync with presets. |
| `presets/.local/MODELS-notes.md` | Local rationale, memory estimates, tuning (gitignored). |
| `skills/` | Qwen Code / Superpowers skills (e.g. `spec-specialist-review`). |
| `completions/_ai-mode` | zsh completion. Update when you add/rename commands or flags. |

## Role map (dev-shop)

- **chief** (`Qwen3-30B-A3B-Instruct-2507`, **131k**, test swap) — **sole user-facing POC**; clarifies, synthesizes,
  coordinates Superpowers flows. Routes via **jev**; delegates monorepo exploration and
  plans to **architect** (human stays on chief).
- **fast** (`Qwen3-4B`, 16k) — cheap instruct chat for classifiers, structured JSON, and
  quick side-queries (Qwen Auto-mode / internal tooling). **Not** a jev Stage A specialist;
  call directly via `/v1/chat/completions`. In Qwen Code: `/model --fast fast` (set
  `contextWindowSize` → **16384** in `~/.qwen/settings.json`).
- **jev** (`Kev-4B`, 16k) — System One router. **Not a chat model.** Two-stage calls:
  (A) `questions.role` → specialist; (B) when role is `qa`, `questions.test_layer` →
  which guideline to load (`presets/agents/jev.md`). Never mix role and layer in one
  question. Needs llama.cpp `/v1/systemone` (`ai-mode doctor`).
- **architect** (`Gemma-4-26B-A4B`, 262144) — **owns planning**, ADRs, and
  large codebase mapping. All multi-step design lives here, not chief. Non-Qwen on purpose so it
  is independent of the coder.
- **coder** (`Qwen3.8-27B`, 131k, thinking on) — agentic executor of architect's plans; chosen over
  Devstral Small 2, Gemma 4, Laguna-S-2.1 and Kimi-Linear in the 2026-10 bake-off (see `presets/.local/BAKEOFF-notes.md`).
  **`qa`** runs the same Qwen3.8 GGUF (shared weights) and works from requirements + guidelines, not the plan.
- **coder-xl** (local `Qwen3-Coder-Next` UD-Q8, 131k) — heavy coder for rare
  under-specified / large jobs; already on disk.
- **specialists** — `product`, `research`, `docs`, `qa`, `security`, `design`, `ux`.
  One domain each; see their `agents/*.md`. **`qa`** authors tests using layer(s) from
  jev Stage B; guidelines live in `presets/agents/qa/guidelines/` (skill:
  `skills/testing-guidelines/SKILL.md`).

Rules of the road:
- **Planning is architect's job.** chief must not write implementation plans.
- **chief routes via jev**, then synthesizes specialist output — don't forward walls of text.
- **Never dump monorepo-wide explorations on chief.** Route large reads → **architect** (262k).

## Superpowers pipeline (Qwen Code)

For test design in Superpowers/Qwen Code: chief routes to **qa** via jev Stage A, runs
jev Stage B for `test_layer`, then qa loads the matching guideline file(s) before authoring.

Architectural execution runs on **architect** (262k); **chief** orchestrates — you do not
need to switch Qwen Code models for routing.

1. **brainstorming** (chief coordinates; architect runs large exploration) →
   `docs/superpowers/specs/…-design.md`; you approve it for review.
2. **`/superpowers:spec-specialist-review`** → sequential passes: product → research →
   security → ux → design → architect. Each emits Blocking / Non-blocking / Proposed edits.
3. **chief merges** reviews into the spec; you re-approve.
4. Chief routes **`drafting-plans`** to **architect**.

Skill sources (symlinked into `~/.qwen/skills/`; Superpowers itself is disabled for now, and routing rules come from `ai-mode setup-qwen`):

- [`skills/spec-specialist-review/SKILL.md`](skills/spec-specialist-review/SKILL.md)
- [`skills/ai-mode-routing/SKILL.md`](skills/ai-mode-routing/SKILL.md) — ad-hoc “ask product/security/…” from chief

## CLI must-knows

Verify any flag against `internal/app/` (or `ai-mode <cmd> -h`) before relying on it.

- `ai-mode doctor` — install/profile/health check. Also flags roles whose running `n_ctx_slot` (from the
  llama-server log) differs from the preset `ctx-size` (model-trained-ctx cap) and presets edited after
  start (`ai-mode restart` applies them; `use` on an active profile does not). Confirms `/v1/systemone` support, agent
  coverage, port clashes, PATH. **Run this first when something is off.**
- `ai-mode use <profile>` — stop any managed server, start the profile, then warm the
  `warm =` list. Flags: `--force`, `--timeout`, `--no-warm`, `--warm-timeout`.
- `ai-mode warm [roles…]` — prefetch/load on the active router (sequential, safe for
  `models-max`). Defaults to `warm =` (dev-shop: chief,jev,architect,coder,fast; else chief,jev). `--all` warms every ini
  section (large downloads). `--warm-timeout <s>` per model.
- `ai-mode prompt <role>` — print a role's system prompt (body only; `--raw`/`--json` for more).
- `ai-mode classify <role|layer> "<task>"` — jev routing (Stage A role / Stage B test layer) with a clear/ambiguous/low-confidence decision; traced. Roles need a `summary:` in `agents/<role>.md` to be routable (`chief`, `coder-xl`, `fast`, `jev` deliberately have none). `classify --eval evals/classify.jsonl` / `make eval` measures routing; tune summaries against it.
- `ai-mode team "<topic>" [--context PATH]... [--roles a,b,c] [--max-tokens N] [--out FILE]` — sequential multi-role consult (default architect → product → research → ux → security; architect's findings feed the rest; roles absent from the profile are skipped). One report on stdout, saved under the state dir's `team/`. Chief's first move for open-ended "whole team" asks.
- `ai-mode amigos <story...> [--story FILE|-] [--rounds N] [--audit off|summary|full] [--out FILE] [--resume ID] [--json] [--caller C]` — Three Amigos refinement of one story (product, qa, architect): prints the verdict, layer plan, open questions and the Story Contract; exit 0 ready, 2 not-ready, 3 paused (needs a human), 1 error; state under `amigos/<meeting_id>/` in the state dir. Wrapped by `skills/three-amigos`.
- `ai-mode setup-qwen [--file PATH] [--settings PATH] [--print] [--remove]` — install/refresh the routing rules (source: `internal/app/qwen_block.md`) as a marked block in `~/.qwen/QWEN.md`, which Qwen Code loads in every session, so `team`/`ask`/`classify` routing doesn't depend on any skill being invoked. Also adds a `SessionStart` hook (named `ai-mode`) to `~/.qwen/settings.json` that runs `ai-mode hook session-start`, which injects `internal/app/using_ai_mode.md` while a server is running: chief's first output must be a real tool call, and open-ended asks go `skill` → `ai-mode-routing` → `ai-mode team`. Without it chief copies the text-form example calls in Qwen Code's built-in prompt and nothing runs (replay of a captured failure: 0/5 real calls without, 10/10 with). Idempotent; keeps your own text and hooks. Re-run after editing either file.
- `ai-mode plan lint <plan.md> [--json]` — check a `skills/drafting-plans` document: sections, JSON task graph (waves are computed and printed), impl tasks depend only on contract tasks, no ownership overlap between parallel tasks, hot files owned by one integration task, every scenario covered by exactly one task with the id in the test name, each packet using the promised test names, and every quoted spec excerpt appearing verbatim in the spec named by the `**Spec:**` header. `--skeleton` skips the per-packet checks. Exit 0 clean, 2 findings.
- `ai-mode plan verify <plan.md> --task Tnn|all [--repo DIR] [--json]` — on a temporary copy of the repo, apply a **code** packet's ancestors, then Phase A (must compile and fail on an assertion), then Phase B (gofmt, `go vet`, green, full `go test ./...`). Contract tasks skip the red step; spec packets report `n/a`. Exit 0 verified/n/a, 2 failed. `make plan-example` runs both on the worked example.
- `ai-mode plan apply <plan.md> --task Tnn [--phase A|B|all] [--repo DIR]` — write a task's file blocks into the repo exactly as the packet gives them (no judgment calls; executors then run the packet's `Run:` command). Exit 0 written, 1 error.
- `ai-mode ask <role> "<question>"` — one-shot chat to a role (system prompt + user message; `--max-tokens`, `--profile`, pipe stdin when no message argument is given). **Roles are plain chat models with no file or shell tools: they only see the prompt.** Ground them with `--context PATH` (repeatable; a file's contents or a directory's file listing; secret-looking files and binaries refused; over `--context-max-chars`, default 200000, is an error). Never tell a role to "read" or "explore" a path without supplying it.
- `ai-mode stop [--force]` — stop the managed server.
- `ai-mode use <profile> --debug` — front the server with a logging proxy on the public port (llama-server moves to port+10000) so Qwen/other clients' chat traffic shows up in `events`/`stats`/`review`. Off by default; `use <profile>` without the flag returns to normal. `restart` keeps the mode; `doctor` flags a dead proxy.
- Observability: `events`, `trace`, `stats`, `ps`, and `review` (label real calls; `--export` → eval cases). See README.

Also: `list`, `which`/`status`, `restart`, `logs [-f] [-n] [--role NAME] [--no-annotate]`, `url`, `models`, `env`,
  `agents`, `init <name> [--with-ini] [--port] [--models-max]`.

Shell env: `eval "$(ai-mode env)"` exports `OPENAI_BASE_URL` for the active profile.

## Qwen Code (local OpenAI providers)

Each ai-mode role is a custom model in `~/.qwen/settings.json` under
`modelProviders.openai[]` (`baseUrl` → `http://127.0.0.1:<port>/v1`, `id` = role name).

Qwen Code does **not** infer context from llama-server’s `meta.n_ctx` for these aliases. If
`contextWindowSize` is missing, the UI defaults to **200k** (`DEFAULT_TOKEN_LIMIT`) while
llama-server still enforces each role’s `ctx-size` from the preset ini — e.g. chief **131072**,
which produces `Context size has been exceeded` near the real limit with a misleading
“200k · 15% used” bar if `contextWindowSize` is wrong.

**Known issue (open):** a fresh Qwen Code session on `chief` still showed a **200.0k** context bar (2026-10-09) even though
`~/.qwen/settings.json` has `contextWindowSize` = 131072 on the `chief` entry. The session header read "API Key | chief", so
the session may not be resolving the model through `modelProviders`. Until fixed, the percent-used bar and auto-compaction
(measured against the window) are wrong for chief. Not yet diagnosed.

Set **`contextWindowSize`** on every local role entry to match `presets/<profile>.ini`
(`chief` → **131072**, `architect` → **262144**, `coder` → **131072**, etc.; see `presets/MODELS.md`). Restart or reload
Qwen Code after editing. Stay on **chief** in the UI; routing to architect/coder is orchestration-side.

**Compaction (Qwen Code 0.25):** auto-compaction is on by default and is measured against each entry's `contextWindowSize`,
so keep those accurate. `~/.qwen/settings.json` sets `context.autoCompactThreshold` = 0.6 (default 0.85 is late for Gemma, which
was reliable to ~127k and degraded near 246k) and `compactionModel` = `architect` (fast MoE, 262k window; `fast` is 16k and too small
to summarize a long history). Manual: `/compress`, `/compress-fast`.

Don't put `enable_thinking` in a provider's `extra_body`: llama-server ignores it at the top level of the
request, so it only looks like a toggle. Thinking is set per role in the preset ini via
`chat-template-kwargs`. Only chat roles belong here (not `jev`, a router) and allow no Qwen
`Agent(...)` subagents for specialist consults.

**Open-ended asks** ("the whole team", "what's possible?"): chief runs **`ai-mode team "<ask>"`** first and may make at most 3 read-only tool calls before delegating (see `presets/agents/chief.md`).

**Ad-hoc specialists:** When the human asks chief to consult **product**, **security**, or another role, chief must run **`ai-mode ask <role> "<question>"`** (or briefly `/model <role>`) — not Qwen `Task`/`Agent` subagents and not re-invoking **`using-superpowers`** as a delegate. Skill: [`skills/ai-mode-routing/SKILL.md`](skills/ai-mode-routing/SKILL.md) (symlinked like `spec-specialist-review`).

## Preset edit rules

- **ini keys must be valid llama-server preset options.** The key is `load-mode` (value
  `mmap`), **not** a key named `mmap`. Don't invent keys.
- **Every ini `[section]` needs a matching `agents/<role>.md`** (frontmatter `role:` must
  match). `ai-mode doctor` flags missing agents.
- **Keep `presets/MODELS.md` in sync** with the ini — catalog entries must match `[section]` blocks.
  Rationale and memory debugging go in `presets/.local/MODELS-notes.md`.
- `models-max` caps resident models; warming still prefetches the rest. dev-shop uses
  `models-max = 6` with **`warm = chief,jev,architect,coder,fast`** so orchestration, plan→execute,
  and Qwen Auto-mode classifiers (~77 GiB wired measured at warm, ~81 GiB with `qa` loaded) are resident on 128 GB.
  The sixth slot is `qa`: it runs the coder's Qwen3.8 GGUF (weights shared via mmap), so calling it adds ~2 GiB and
  does not evict chief.
  Don't pair a ~60 GiB architect (e.g. Laguna-S-2.1) with the other four: wired memory hit ~110 GiB and swap started.
- Thinking models (architect's Gemma 4, coder's Qwen3.8) keep thinking **on** — do **not** set
  `enable_thinking=false` for them.
  Cap runaway thinking with `reasoning-budget` (a valid preset key; architect 8192, coder 16384) instead.

## Model selection

- **Prefer official GGUF** (`Qwen/…`, `ggml-org/…`) when a trusted quant exists.
- Where no official GGUF exists (Thinking-2507, Coder-30B-A3B), use trusted non-Unsloth
  quantizers (bartowski / lmstudio-community / ggml-org). The earlier Unsloth
  Qwen3-30B-A3B degeneration bit us — avoid it for those.
- architect = `Gemma-4-26B-A4B` @ ctx `262144`. In the end-to-end bake-off (architect spec → coder → QA-test fix loop) Gemma
  specs + Qwen3.8 coder gave 6/6 fully-correct cells; Qwen3.8 as its own architect scored only 0.50 (over-specified or empty
  specs). Laguna-S-2.1 (~60 GiB) and Kimi-Linear did not improve any role.

## Guardrails

- **Don't commit or push unless the user explicitly asks.**
- **Don't invent CLI flags** — confirm in `internal/app/`.
- Don't expose the API beyond `127.0.0.1` without care; weights can be large downloads.
  dev-shop deliberately uses `host = 0.0.0.0` so the web UI is reachable from other Macs on the LAN **and**
  Qwen Code's `127.0.0.1` baseUrls keep working. Don't bind to a specific LAN IP: that drops loopback (breaking
  Qwen Code) and fails to start if DHCP hands out a new address.
- Keep `README.md`, `MODELS.md`, `agents/*.md`, and `completions/_ai-mode` consistent when
  you change roles or commands.
