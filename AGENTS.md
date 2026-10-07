# AGENTS.md

Guidance for anyone (human or LLM) working **in this repo**. For end-user usage,
see [`README.md`](README.md). For model rationale, see [`presets/MODELS.md`](presets/MODELS.md).

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
| `bin/ai-mode` | The CLI (single Python 3 file, stdlib only). Source of truth for commands/flags. |
| `presets/*.ini` | llama-server model presets. `[section]` = a role; `[*]` = shared defaults. |
| `presets/*.mode` | Sidecar metadata: `port`, `models-max`, `host`, `description`, `warm`. |
| `presets/agents/*.md` | System prompt per role. Frontmatter `role:` must match an ini `[section]`. |
| `presets/MODELS.md` | Why each model/ctx/sampling was chosen. Keep in sync with the ini files. |
| `skills/` | Qwen Code / Superpowers skills (e.g. `spec-specialist-review`). |
| `completions/_ai-mode` | zsh completion. Update when you add/rename commands or flags. |

## Role map (dev-shop)

- **chief** (`Qwen3-14B`, **131k**) — **sole user-facing POC**; clarifies, synthesizes,
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
- **architect** (`Qwen3-30B-A3B-Thinking-2507`, 262144) — **owns planning**, ADRs, and
  large codebase mapping. All multi-step design lives here, not chief.
- **coder** (`Qwen3-Coder-30B-A3B`, 131k) — agentic executor of architect's plans.
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
4. Chief routes **`writing-plans`** to **architect**.

Skill source: [`skills/spec-specialist-review/SKILL.md`](skills/spec-specialist-review/SKILL.md),
symlinked into `~/.qwen/extensions/superpowers/skills/`.

## CLI must-knows

Verify any flag against `bin/ai-mode` before relying on it.

- `ai-mode doctor` — install/profile/health check. Confirms `/v1/systemone` support, agent
  coverage, port clashes, PATH. **Run this first when something is off.**
- `ai-mode use <profile>` — stop any managed server, start the profile, then warm the
  `warm =` list. Flags: `--force`, `--timeout`, `--no-warm`, `--warm-timeout`.
- `ai-mode warm [roles…]` — prefetch/load on the active router (sequential, safe for
  `models-max`). Defaults to `warm =` (dev-shop: chief,jev,architect,coder,fast; else chief,jev,architect). `--all` warms every ini
  section (large downloads). `--warm-timeout <s>` per model.
- `ai-mode prompt <role>` — print a role's system prompt (body only; `--raw`/`--json` for more).
- `ai-mode stop [--force]` — stop the managed server.
- Also: `list`, `which`/`status`, `restart`, `logs [-f] [-n] [--role NAME] [--no-annotate]`, `url`, `models`, `env`,
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

Set **`contextWindowSize`** on every local role entry to match `presets/<profile>.ini`
(`chief` → **131072**, `architect` → **262144**, `coder` → **65536**, etc.; see `presets/MODELS.md`). Restart or reload
Qwen Code after editing. Stay on **chief** in the UI; routing to architect/coder is orchestration-side.

## Preset edit rules

- **ini keys must be valid llama-server preset options.** The key is `load-mode` (value
  `mmap`), **not** a key named `mmap`. Don't invent keys.
- **Every ini `[section]` needs a matching `agents/<role>.md`** (frontmatter `role:` must
  match). `ai-mode doctor` flags missing agents.
- **Keep `presets/MODELS.md` in sync** with the ini — model choice, ctx, and sampling
  rationale live there.
- `models-max` caps resident models; warming still prefetches the rest. dev-shop uses
  `models-max = 5` with **`warm = chief,jev,architect,coder,fast`** so orchestration, plan→execute,
  and Qwen Auto-mode classifiers (~98 GiB) are resident at startup on 128 GB.
- Thinking models (architect's Thinking-2507) keep thinking **on** — do **not** set
  `enable_thinking=false` for them.

## Model selection

- **Prefer official GGUF** (`Qwen/…`, `ggml-org/…`) when a trusted quant exists.
- Where no official GGUF exists (Thinking-2507, Coder-30B-A3B), use trusted non-Unsloth
  quantizers (bartowski / lmstudio-community / ggml-org). The earlier Unsloth
  Qwen3-30B-A3B degeneration bit us — avoid it for those.
- architect = `Qwen3-30B-A3B-Thinking-2507` @ ctx `262144` (official-only fallback:
  `Qwen/Qwen3-32B-GGUF` @ 65536, YaRN beyond 32k).

## Guardrails

- **Don't commit or push unless the user explicitly asks.**
- **Don't invent CLI flags** — confirm in `bin/ai-mode`.
- Don't expose the API beyond `127.0.0.1` without care; weights can be large downloads.
- Keep `README.md`, `MODELS.md`, `agents/*.md`, and `completions/_ai-mode` consistent when
  you change roles or commands.
