# ai-mode

System-wide CLI to switch **llama-server** profile presets and load per-role
**agent** system prompts.

| Profile | Port | Focus |
| --- | --- | --- |
| `dev-shop` | 8080 | Chief + Kev-4B jev router + coding/product/QA/UX specialists |
| `learning-center` | 8081 | Chief + jev → research / notes / podcast script |
| `av-club` | 8082 | Chief + jev → photo/video language (ComfyUI renders pixels) |

Orchestration: **`chief`** (chief of staff) talks to you; **`jev`** (`ggml-org/Kev-4B-GGUF`) classifies via `/v1/systemone`; specialists execute.

Model catalog: [`presets/MODELS.md`](presets/MODELS.md). Rationale and memory notes: [`presets/.local/MODELS-notes.md`](presets/.local/MODELS-notes.md) (local).  
Role system prompts live in [`presets/agents/`](presets/agents/).  
Working in this repo (human or LLM)? Read [`AGENTS.md`](AGENTS.md).

## Install

```bash
./install.sh
# builds dist/ai-mode (needs Go), links ~/.local/bin/ai-mode, and points
# ~/.config/ai-mode/config at this repo's presets/
```

### Build

ai-mode is a single static Go binary (stdlib only, no runtime or environment setup).
Needs Go to build (`brew install go`).

```bash
make build      # → dist/ai-mode
make test
make install    # symlinks dist/ai-mode to ~/.local/bin/ai-mode
```

#### Logging, traceability, observability 

Every `ask`, `use`, `stop`, `restart` and `warm` appends a JSON line to
`~/.local/state/ai-mode/events.jsonl` (rotated at 10 MiB). Each `ask` records latency,
token counts, gen/prompt tok/s, cached tokens, finish reason, whether the model was
cold-started, who asked, and the full request/response in `traces/<trace>/<span>.json`
(kept 14 days; `AI_MODE_CAPTURE=0` disables, `AI_MODE_RETAIN_DAYS=N` changes retention).

```bash
ai-mode events [-f] [--kind ask|classify] [--role R] [--status error] [--since 1h] [--json]
ai-mode stats [--since 24h] [--role R]     # p50/p95, tokens, tok/s, errors, truncations, warm times
ai-mode trace [ID] [--full]                # call tree for a trace (default: latest); --list for recent
ai-mode ps                                 # roles loaded/unloaded, ctx, last ask
```

`ai-mode classify --eval evals/classify.jsonl [--min-accuracy 0.9]` scores jev's routing against
labelled cases (accuracy, confidence, wrong-but-clear, confusions) without writing events; `make eval`
runs the tuning set and the held-out set. See *Tuning jev's confidence* in
[`presets/agents/jev.md`](presets/agents/jev.md).

**Review real traffic.** `ai-mode review` steps through recent calls (default: last 7 days, newest 20, only
ones you haven't judged) and asks how each did:

```bash
ai-mode review                       # interactive: Enter = looks right, or type the correct role / n [why]
ai-mode review --list                # how many are waiting
ai-mode review --kind classify --since 24h --role qa
ai-mode review --export evals/reviewed.jsonl   # reviewed routing calls → eval cases (appends, de-duplicated)
ai-mode classify --eval evals/reviewed.jsonl   # score jev against what you actually decided
```

Verdicts live in `~/.local/state/ai-mode/reviews.jsonl` and show up in `ai-mode stats` (ok / wrong /
wrong-but-clear per role) and `ai-mode trace`. `evals/reviewed.jsonl` holds real task text, so it is
gitignored. To check chief is using the router at all: `ai-mode events --kind classify --since 24h`.

To link a chain (chief → specialist → specialist) into one trace, pass ids along:
`T=$(ai-mode trace new); ai-mode ask product "..." --trace $T --caller chief -v` prints
`span=<id>`; use `--parent <id>` on follow-ups (or set `AI_MODE_TRACE_ID`,
`AI_MODE_PARENT_SPAN`, `AI_MODE_CALLER`). Calls flagged `⚠` were truncated by `max_tokens`
or hit a cold model.

Presets are found via `$AI_MODE_PRESETS`, `~/.config/ai-mode/config`, or `<repo>/presets`
next to the symlink-resolved binary.

## Usage

```bash
ai-mode list
ai-mode use dev-shop
ai-mode which
ai-mode doctor

ai-mode agents                 # role guides
ai-mode prompt product         # system prompt for a role
ai-mode ask product "What is a user story in one sentence?"
ai-mode classify role "Write edge-case tests for login rate limiting"   # jev picks the specialist
ai-mode classify layer "Add tests for coupon validation"                 # jev picks the QA test layer
eval "$(ai-mode env)"          # OPENAI_BASE_URL for this shell
```

Also: `stop`, `restart`, `logs [-f] [-n] [--role NAME] [--no-annotate]`, `url`, `models`, `ask`, `init <name> --with-ini`.

**Prefetch (avoid first-request HF downloads):** After the router is healthy, `ai-mode use dev-shop` sequentially warms the profile’s `warm = …` list (dev-shop default: `chief`, `jev`, `architect`, `coder`, `fast`). Each role is probed via `/v1/chat/completions` or `/v1/systemone` until its weight is downloaded/loaded. Override with `warm = …` in `<profile>.mode`, `ai-mode warm chief architect`, `ai-mode warm --all` (every ini section — large downloads), or skip with `--no-warm`. With dev-shop `models-max = 5`, those five models stay resident (~98 GiB); warming still prefetches other roles for the next request. Tune per-model wait with `--warm-timeout 900`.

## Adding a profile later

1. `ai-mode init my-lab --with-ini --port 8083`
2. Edit `presets/my-lab.ini` (models + ctx/sampling)
3. Add `presets/agents/<role>.md` for each `[section]`
4. Add catalog rows in `presets/MODELS.md`; optional rationale in `presets/.local/MODELS-notes.md`

## Notes

- First `hf = …` request downloads into `~/.cache/llama.cpp` (can be large). Use `ai-mode warm` or let `ai-mode use` prefetch orchestration models.
- VL roles auto-pull `mmproj` when the HF repo includes one.
- For `design` / `ux` screenshots, user text should include: `Critique this UI design.`
- `qa` carries a guideline router in its prompt and loads vendored testing guides on demand from [`presets/agents/qa/guidelines/`](presets/agents/qa/guidelines/) (one layer at a time to fit its 65k ctx).
- Podcast **audio** and **image/video generation** are outside llama-server (TTS / ComfyUI).

## Superpowers pipeline (Qwen Code)

Stay on **`chief`** in Qwen Code (dev-shop **131k** ctx) — chief routes exploration and
plans to **`architect`** (262k) via jev; you should not need `/model` switches for Superpowers.

If the status bar shows **200k** context for `chief` but requests fail with **Context size has
been exceeded** before the real limit, set `contextWindowSize` on each `modelProviders.openai[]`
entry in `~/.qwen/settings.json` to match `ctx-size` in the active preset (chief → **131072**,
**fast** → **16384**). See `AGENTS.md` → Qwen Code.

For Qwen Code **Auto-mode** side classifiers, point the fast alias at **`fast`**
(`Qwen3-4B`): add a provider entry with `"id": "fast"` and run **`/model --fast fast`**.

After brainstorming writes `docs/superpowers/specs/…-design.md` and you approve it for review:

1. `/superpowers:spec-specialist-review` — product → research → security → ux → design → architect
2. **`chief`** merges reviews into the spec; you re-approve
3. Chief routes **`/superpowers:writing-plans`** to **`architect`**

Skill source: [`skills/spec-specialist-review/SKILL.md`](skills/spec-specialist-review/SKILL.md) (symlinked into `~/.qwen/extensions/superpowers/skills/`). Brainstorming overlay: backup at `~/.qwen/.../brainstorming/SKILL.md.bak`.
