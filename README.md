# ai-mode

System-wide CLI to switch **llama-server** profile presets and load per-role
**agent** system prompts.

| Profile | Port | Focus |
| --- | --- | --- |
| `dev-shop` | 8080 | Chief + Kev-4B jev router + coding/product/QA/UX specialists |
| `learning-center` | 8081 | Chief + jev → research / notes / podcast script |
| `av-club` | 8082 | Chief + jev → photo/video language (ComfyUI renders pixels) |

Orchestration: **`chief`** (chief of staff) talks to you; **`jev`** (`ggml-org/Kev-4B-GGUF`) picks the specialist via `ai-mode classify` (llama-server `/v1/systemone`); specialists execute.

Model catalog: [`presets/MODELS.md`](presets/MODELS.md). Rationale and memory notes: [`presets/.local/MODELS-notes.md`](presets/.local/MODELS-notes.md) (local).  
Role system prompts live in [`presets/agents/`](presets/agents/).  
Working in this repo (human or LLM)? Read [`AGENTS.md`](AGENTS.md).

## Install

Needs Go to build (`brew install go`). ai-mode is a single static binary: stdlib only, no runtime or environment setup.

```bash
./install.sh
# builds dist/ai-mode, links ~/.local/bin/ai-mode, points ~/.config/ai-mode/config at this repo's presets/,
# and prints the exact command to put ~/.local/bin on your PATH if it isn't already
ai-mode doctor
```

Or by hand: `make build` (→ `dist/ai-mode`), `make test`, `make install` (symlink into `~/.local/bin`), `make eval` (see *Routing*).

Presets are found via `$AI_MODE_PRESETS`, `~/.config/ai-mode/config`, or `<repo>/presets` next to the
symlink-resolved binary, then `~/models/presets`.

## Usage

```bash
ai-mode list
ai-mode use dev-shop
ai-mode which
ai-mode doctor

ai-mode agents                 # role guides
ai-mode prompt product         # system prompt for a role
ai-mode ask product "What is a user story in one sentence?"
ai-mode team "web version of the app?" --context internal/app --context README.md   # architect gets a file listing + README, then product/research/ux/security; one report
ai-mode ask architect "Where are commands registered?" --context internal/app/cli.go   # ground a role in real code
ai-mode plan lint docs/superpowers/plans/my-plan.md          # check a drafting-plans document (structure, ownership, scenario coverage)
ai-mode plan verify docs/superpowers/plans/my-plan.md --task all   # apply each code task to a copy of the repo: must be red, then green
eval "$(ai-mode env)"          # OPENAI_BASE_URL for this shell
```

Roles are plain chat models: they cannot open files or run commands, they only see the prompt. `--context PATH` (repeatable, on `ask` and `team`)
puts a **file's contents** or a **directory's file listing** into the prompt as reference material, so answers are grounded in the real repo
instead of guessed. Likely-secret files (`.env*`, `*.pem`, `*.key`, `id_rsa*`) and binaries are refused, `.git`/`node_modules`/`vendor`/`dist`
are skipped, and a context over `--context-max-chars` (default 200000) is an error, never a silent truncation. `team` gives the context to
architect (or the first role if architect is not on the panel); the others get architect's findings.

`team` runs its roles sequentially (architect first; its findings are passed on as shared context), prints one markdown
report and saves it under `~/.local/state/ai-mode/team/`. `--roles a,b,c` changes the panel, `--out FILE` also writes there.
A role that fails is marked `FAILED` in the report; all calls share one trace (`ai-mode trace`).

To make Qwen Code's chief reach for `team`/`ask`/`classify` in any workflow (not only inside a skill), run `ai-mode setup-qwen` once;
it adds a marked routing block to `~/.qwen/QWEN.md` (`--print` shows it, `--remove` undoes it).

Also: `stop`, `restart`, `logs [-f] [-n] [--role NAME] [--no-annotate]`, `url`, `models`, `warm`, `init <name> --with-ini`,
plus `classify`, `events`, `trace`, `stats`, `ps` and `review` below.

**Prefetch (avoid first-request HF downloads):** After the router is healthy, `ai-mode use dev-shop` sequentially warms the profile’s `warm = …` list (dev-shop default: `chief`, `jev`, `architect`, `coder`, `fast`). Each role is probed via `/v1/chat/completions` or `/v1/systemone` until its weight is downloaded/loaded. Override with `warm = …` in `<profile>.mode`, `ai-mode warm chief architect`, `ai-mode warm --all` (every ini section — large downloads), or skip with `--no-warm`. With dev-shop `models-max = 5`, those five models stay resident (~77 GiB wired); warming still prefetches other roles for the next request. Tune per-model wait with `--warm-timeout 900`.

## Routing (jev)

```bash
ai-mode classify role  "Write edge-case tests for login rate limiting"   # Stage A: which specialist
ai-mode classify layer "Add tests for coupon validation"                  # Stage B: which QA test layer
```

Prints `choice`, `confidence`, and a `decision` (`clear` / `ambiguous` / `low-confidence`); `--json` and `--short`
are available. The candidate roles are the profile's ini sections whose `agents/<role>.md` has a `summary:` line
(`chief`, `coder-xl`, `fast`, `jev` deliberately have none). Reword summaries to match how people phrase tasks; measure
the effect rather than guessing:

```bash
make eval                                           # tuning set + held-out set
ai-mode classify --eval evals/classify.jsonl [--min-accuracy 0.9]
```

`--eval` scores accuracy, confidence, wrong-but-clear and confusions without writing events. See *Tuning jev's
confidence* in [`presets/agents/jev.md`](presets/agents/jev.md).

## Observability

Every `ask`, `classify`, `use`, `stop`, `restart` and `warm` appends a JSON line to
`~/.local/state/ai-mode/events.jsonl` (rotated at 10 MiB). Each `ask` records latency, token counts, gen/prompt tok/s,
cached tokens, finish reason, whether the model was cold-started, who asked, and the full request/response in
`traces/<trace>/<span>.json` (kept 14 days; `AI_MODE_CAPTURE=0` disables, `AI_MODE_RETAIN_DAYS=N` changes retention).

```bash
ai-mode events [-f] [--kind ask|classify] [--role R] [--status error] [--since 1h] [--json]
ai-mode stats [--since 24h] [--role R]     # p50/p95, tokens, tok/s, errors, truncations, routing, warm times, your reviews
ai-mode trace [ID] [--full]                # call tree for a trace (default: latest); --list for recent
ai-mode ps                                 # roles loaded/unloaded, effective ctx (! = differs from preset), last ask
```

### Debug mode: record traffic from Qwen Code and other clients

`events`, `stats`, `trace` and `review` only see calls made through the `ai-mode` binary. Qwen Code talks to
llama-server directly, so chief's own turns are invisible by default. To record them, start the profile in
debug mode:

```bash
ai-mode use dev-shop --debug     # a logging proxy takes the public port; llama-server moves to port+10000
ai-mode events -f                # watch chief's chat completions arrive (via=proxy)
ai-mode use dev-shop             # back to normal: no proxy in the path
```

Qwen's `baseUrl` doesn't change. Each chat completion is recorded as an `ask` (`via=proxy`) with latency,
time-to-first-token, tokens, tok/s, tool-call count, finish reason, and a payload that keeps the **last user
message and the reply**, not the whole resent conversation (`AI_MODE_PROXY_CAPTURE=full` keeps everything).
The CLI's own calls aren't double-logged. Streaming passes through unbuffered, and a client disconnect
cancels the upstream request. `which` and `doctor` show the mode and flag a dead proxy; `restart` keeps the
mode; the proxy log is `~/.local/state/ai-mode/logs/<profile>.proxy.log`. Payloads are capped at 500 MB total
(`AI_MODE_TRACES_MAX_MB`), oldest first, on top of the 14-day retention.

Caveats: switching modes restarts the server (and re-warms); requests from Qwen carry no trace ids, so each is
its own trace (clients can send `X-AI-Mode-Trace` / `X-AI-Mode-Parent` headers); the logs hold your prompts and
code in plaintext, so leave debug mode off when you don't need it.

To link a chain (chief → specialist → specialist) into one trace, pass ids along:
`T=$(ai-mode trace new); ai-mode ask product "..." --trace $T --caller chief -v` prints
`span=<id>`; use `--parent <id>` on follow-ups (or set `AI_MODE_TRACE_ID`,
`AI_MODE_PARENT_SPAN`, `AI_MODE_CALLER`). Calls flagged `⚠` were truncated by `max_tokens`,
hit a cold model, or got an unclear routing decision. To check chief is using the router at all:
`ai-mode events --kind classify --since 24h`.

### Reviewing real traffic

`ai-mode review` steps through recent calls (default: last 7 days, newest 20, only ones you haven't judged) and asks how each did:

```bash
ai-mode review                       # interactive: Enter = looks right, or type the correct role / n [why]
ai-mode review --list                # how many are waiting
ai-mode review --kind classify --since 24h --role qa
ai-mode review --export evals/reviewed.jsonl   # reviewed routing calls → eval cases (appends, de-duplicated)
ai-mode classify --eval evals/reviewed.jsonl   # score jev against what you actually decided
```

Verdicts live in `~/.local/state/ai-mode/reviews.jsonl` and show up in `ai-mode stats` (ok / wrong /
wrong-but-clear per role) and `ai-mode trace`. `evals/reviewed.jsonl` holds real task text, so it is gitignored.

## Adding a profile later

1. `ai-mode init my-lab --with-ini --port 8083`
2. Edit `presets/my-lab.ini` (models + ctx/sampling)
3. Add `presets/agents/<role>.md` for each `[section]`; add a `summary:` line (frontmatter) to each role jev should be able to route to
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
3. Chief routes **`/superpowers:drafting-plans`** to **`architect`**

Skill source: [`skills/spec-specialist-review/SKILL.md`](skills/spec-specialist-review/SKILL.md) (symlinked into `~/.qwen/skills/`; Superpowers is disabled for now).
