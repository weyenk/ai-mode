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
# ensures ~/.local/bin/ai-mode and ~/.config/ai-mode/config → this repo's presets/
```

## Usage

```bash
ai-mode list
ai-mode use dev-shop
ai-mode which
ai-mode doctor

ai-mode agents                 # role guides
ai-mode prompt product         # system prompt for a role
ai-mode ask product "What is a user story in one sentence?"
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
