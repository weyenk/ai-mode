# MLX backend (`mlx-community` models) — design notes

Status: **draft, measure-first.** Written 2026-10-11 from a brainstorming request ("convert ai-mode to MLX models from
mlx-community for better token throughput"). Not yet reviewed via `spec-specialist-review`. Facts about third-party servers
below come from their docs/READMEs as of 2026-10-11 and are marked **(verify)** where we have not run them ourselves.

## Goal

Get more tokens per second out of the Mac Studio M5 Max / 128 GB for the roles where it matters (chief, coder, qa,
architect), **without losing anything ai-mode relies on today**: one OpenAI-compatible port per profile, many roles per
port, `models-max`-style residency, warm-up, jev System One routing, per-role sampling and thinking control, context caps
that `doctor` can check, and the bake-off quality we already measured.

"Convert to MLX" is a hypothesis, not a decision. The deliverable of the first phase is numbers on *our* hardware, *our*
models and *our* context sizes. Roles move to MLX only where those numbers say so.

## Why not just switch

Public numbers point both ways (sources at the end):

- **Decode at short/medium context:** MLX is commonly reported 20-40% faster than llama.cpp on Apple Silicon (e.g. Qwen3
  30B-A3B ~110 vs ~90 tok/s on an M4 Max). M5's neural accelerators are used by MLX; whether our llama.cpp build uses them
  is unknown.
- **Prefill on M5:** large MLX gains reported for MoE (Qwen3.5-35B-A3B NVFP4: 1,154 → 1,810 tok/s prefill on M5 Max).
- **Long context flips:** one measurement at 146k tokens had MLX decode at ~6 tok/s vs ~12 tok/s for llama.cpp. Our
  architect runs at **262k** and chief/coder at **131k** — exactly the regime where the MLX lead is least certain.
- **Quantization is not like-for-like.** MLX 4-bit (affine, group size 64) is not Q4_K_M. The coder/qa/architect choices
  came from a graded bake-off (`presets/.local/BAKEOFF-notes.md`); a quant change can move those scores.

So: benchmark, then decide per role.

## What ai-mode depends on from llama-server today

Each row is a requirement any MLX backend must meet, or the role stays on llama-server.

| Dependency | Where in ai-mode | llama-server today | `mlx_lm.server` | oMLX **(verify)** |
| --- | --- | --- | --- | --- |
| Many roles on one port, LRU residency cap | `startProfile` (`--models-preset`, `--models-max`) | router mode | **No** — one model per process; `model` field loads by path/repo | Yes — multi-model, LRU eviction, pinning, memory guard (default RAM − 8 GB) |
| Per-role id → weights + params | `presets/*.ini` sections | ini preset | n/a (request params only) | per-model settings + **aliases** in `~/.omlx/settings.json` (schema undocumented) |
| `/v1/models` with load status | `fetchCatalog`, `modelStatus` (`warm.go`), health checks | `status.value` | id/created only | lists models; status field unknown |
| `/v1/systemone` + Kev-4B for jev | `classify.go`, `warm.go`, `doctor.go` | patched llama.cpp | **No** | endpoint exists, but listed decision models are Clef / Clef-Flash / OpenJev; **no MLX Kev-4B found** |
| Per-role thinking toggle | `chat-template-kwargs` in ini | yes | not documented | per-model chat template kwargs |
| Thinking cap (`reasoning-budget` + message) | coder 16384, architect 8192 | yes | **No** | `thinking_budget` mentioned; parity with the "answer now" message unknown |
| KV cache quantization | chief q4_0; coder/qa/architect q8_0 | yes | `--kv-bits` (disables batching) | not documented |
| Context cap per role + `n_ctx_slot` check | `ctx.go`, `doctor` | `ctx-size`, log line | no cap flag | "reports real context window"; no cap flag documented |
| YaRN for product (32k → 64k) | `[product]` rope keys | yes | via model `config.json` `rope_scaling` | same (config edit) |
| Shared weights coder ↔ qa | same GGUF, mmap | yes | n/a | one model + per-model **profiles** (`<model>:<profile>`) — would replace the shared-GGUF trick |
| Vision (design/ux mmproj, av-club frame-review) | `[design]`, `[ux]` | mmproj | no (mlx-vlm) | VLM support |
| Local non-HF weights (coder-xl UD-Q8 GGUF) | `model = /Users/...gguf` | yes | needs `mlx_lm.convert` from safetensors | same |
| Logs annotated per role | `logs.go` (`[<port>]` prefix) | router log | n/a | `~/.omlx/logs/server.log`, format undocumented |
| LAN bind (`host = 0.0.0.0`) | dev-shop `.mode` | yes | yes | **refuses non-loopback without an API key** → Qwen Code entries need `apiKey` |
| Debug proxy | `proxy.go` (`--debug`) | backend-agnostic | works | works |

Conclusion from the table: `mlx_lm.server` alone cannot replace the router; it is useful as a **benchmark reference**.
oMLX is the only candidate found that matches the router shape, and several of its rows are unverified. jev has no MLX
path today, so **jev stays on llama-server** unless OpenJev passes `make eval`.

## Proposed design

### Phase 0 — measure (`ai-mode bench`), no behavior change

New command, backend-agnostic because it only speaks the OpenAI API:

```
ai-mode bench <role>... [--base-url URL] [--ctx 4k,32k,128k] [--gen 512] [--runs 3] [--json] [--out FILE]
```

- For each role and context size: send a fixed synthetic-plus-real prompt (repo files via the existing `--context`
  machinery so prompts look like ours), stream the reply, record **TTFT, prefill tok/s, decode tok/s, peak wired memory**
  (sampled from `vm_stat`/`footprint` of the server pid), and `usage` token counts.
- Runs against the active profile by default, or any `--base-url` (so the same command measures an `mlx_lm.server` or
  oMLX instance started by hand). Results land in the state dir under `bench/` and as `bench_*` events for `stats`.
- Matrix for the first run (dev-shop): chief, coder, architect, fast at 4k / 32k / 128k (architect also 250k), GGUF
  current preset vs the matching `mlx-community` 4-bit build. Thinking off for the measurement (fixed output length).
- **Quality re-check:** rerun the existing bake-off cells for coder, qa and architect on the MLX builds (spec → coder →
  QA-mutant loop). Throughput gains do not count if the bake-off score drops.

**Go/no-go per role:** move a role only if, at its working context, decode tok/s improves ≥ 20%, TTFT is not worse,
peak memory fits the dev-shop warm set (≤ ~85 GiB wired with `qa` loaded), and its bake-off/eval score does not drop.
Record the numbers in `presets/.local/MODELS-notes.md` and the decision in `MODELS.md`.

### Phase 1 — backend abstraction in ai-mode

Only if Phase 0 says at least one heavy role should move.

- `.mode` gains `backend = llama` (default, today's behavior) or `backend = omlx`. A profile has **one primary backend**.
- New file `internal/app/backend.go`: an interface the existing code calls instead of llama-specific helpers —
  `Start(profile, port)`, `Stop`, `Catalog()` (id → loaded/loading/unloaded), `Load(id)`, `EffectiveCtx()` (for
  `doctor`), `LogLines()` (for `logs` role annotation), `Doctor()` (binary found, version, feature probes).
  `llamaBackend` wraps today's code unchanged; `omlxBackend` is new.
- **Presets stay the source of truth.** For an omlx profile, ai-mode generates the server's per-model config (aliases =
  role names, sampling, chat-template kwargs, thinking budget, TTL, pins from `warm =`) from `presets/<profile>.ini`
  into the state dir, the same way the ini drives llama-server today. MLX roles use a new key `mlx = <hf repo>` in place of
  `hf =`; a section may carry both so switching backends does not lose the GGUF choice. Keys with no MLX equivalent
  (e.g. `cache-type-k` if oMLX has no KV quant) make `doctor` warn rather than silently drop.
- `models-max` maps to the oMLX model cap / memory guard; `warm` maps to pinned models.

### Phase 2 — hybrid profile (expected end state)

Because jev (and possibly security's CyberStag and coder-xl's local GGUF) have no MLX build, the realistic end state is
**two backends behind one port**:

- `backend = hybrid`: llama-server runs the GGUF roles on `port+10000`, oMLX runs the MLX roles on `port+20000`, and the
  existing `proxy.go` (already used by `--debug` to front the public port) routes by the request's `model` field and by
  path (`/v1/systemone` → llama). `/v1/models` is the merged catalog.
- Clients (Qwen Code, `ask`, `team`, `amigos`, `classify`) keep one base URL and see no change.
- Each section declares its backend implicitly (`mlx =` → MLX, `hf =`/`model =` → llama); `ai-mode which` shows the
  backend per role.
- Memory: two servers no longer share one `models-max`. `doctor` sums resident sets of both and warns past the budget.

## Candidate model mapping (dev-shop)

Repo existence checked by search on 2026-10-11 where marked ✓; everything else must be checked before Phase 0.

| Role | Today (GGUF) | MLX candidate | Notes |
| --- | --- | --- | --- |
| chief | `lmstudio-community/Qwen3-30B-A3B-Instruct-2507-GGUF:Q4_K_M` | `mlx-community/Qwen3-30B-A3B-Instruct-2507-4bit` (verify) | MoE; where MLX gains are reported largest |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | none found | stays on llama-server; OpenJev on oMLX only if it passes `make eval` |
| fast | `Qwen/Qwen3-4B-GGUF:Q4_K_M` | `mlx-community/Qwen3-4B-4bit` (verify) | low priority; already fast |
| coder | `ggml-org/Qwen3.8-27B-GGUF:Q4_K_M` | ✓ `mlx-community/Qwen3.8-27B-4bit`; ✓ `mlx-community/Qwen3.8-27B-MTP-4bit` | dense 27B decode is memory-bandwidth bound; the MTP build may allow speculative decoding — benchmark both |
| qa | same as coder | same as coder, as an oMLX profile of the coder model | keeps the shared-weights property |
| architect | `lmstudio-community/gemma-4-26B-A4B-it-GGUF:Q4_K_M` | ✓ `mlx-community/gemma-4-26b-a4b-it-4bit`; ✓ `…-it-qat-4bit` | **262k context is the riskiest case**; a tool-calling bug was reported with mlx_lm 0.31.2 (ml-explore/mlx-lm#1125) |
| coder-xl | local Qwen3-Coder-Next UD-Q8 GGUF | convert from safetensors with `mlx_lm.convert` (8-bit) | rare use; stay on llama unless trivial |
| product / research / docs | Qwen3-14B, Qwen3-30B-A3B-Thinking, gemma-3-27b | `mlx-community/*-4bit` (verify) | product's YaRN needs a `rope_scaling` check |
| security | `deer-sec/CyberStag-…-GGUF` | none known | stays on llama |
| design / ux | `stefans71/frontend-design-expert-8b` (VL) | none known | stays on llama |

learning-center and av-club are out of scope until dev-shop has results (they are smaller models where the gain matters
less; av-club also has VL and fine-tunes without MLX builds).

## Non-goals

- No cloud models; everything stays on the Mac Studio.
- Not removing llama-server. It remains the default and the jev host.
- Not changing role assignments or prompts. A model swap that changes quality is a separate bake-off.
- Not adopting Ollama's or LM Studio's MLX backends: both would add a second model store and GUI/daemon we don't manage.
  (LM Studio's `lms` CLI can serve MLX + GGUF side by side; listed under Open as a fallback if oMLX fails verification.)

## Security

- oMLX binds loopback by default and refuses non-loopback without an API key. dev-shop's `host = 0.0.0.0` therefore needs a
  key: generate it into the state dir (0600), never into the repo, and have `setup-qwen` write it into the Qwen Code
  provider entries. Alternative: oMLX stays on loopback and only the ai-mode proxy binds 0.0.0.0.
- oMLX ships an `/admin` panel that can change settings and load models. It must not be reachable from the LAN: proxy
  only `/v1/*`.
- MLX weights are safetensors (no pickle), but conversion repos are community uploads: prefer `mlx-community` and
  `lmstudio-community`, pin a revision in the preset, and record the hash like the GGUF choices.
- oMLX auto-update (app build) must be off for the managed server; pin the version ai-mode tested.

## Repo changes implied

- `internal/app/bench.go` (+ test), `cli.go` registration, `completions/_ai-mode`, README section — Phase 0.
- `internal/app/backend.go`, `backend_llama.go`, `backend_omlx.go`; `startProfile`, `warm.go`, `ctx.go`, `logs.go`,
  `doctor.go` call through the interface — Phase 1.
- `proxy.go` model-based routing + merged `/v1/models` — Phase 2.
- `presets/*.ini` `mlx =` keys, `presets/*.mode` `backend =`, `MODELS.md` columns for backend and MLX repo,
  `AGENTS.md` preset rules (new keys are ai-mode keys, not llama-server keys — the "keys must be valid llama-server
  options" rule needs an explicit exception), `ai-mode init` template.

## Open

1. Does oMLX's `/v1/models` expose load status, and can ai-mode drive its per-model config headlessly (file or admin API)
   without the menu-bar app? Decides Phase 1 feasibility.
2. Does oMLX support KV quantization and a hard context cap per model? Without q8_0 KV, architect at 262k may not fit the
   warm set.
3. Is oMLX's `thinking_budget` equivalent to `reasoning-budget` + `reasoning-budget-message` (the cap that stopped
   Qwen3.8/Gemma runaway thinking in the bake-off)?
4. Kev-4B: can it be converted to MLX and served by oMLX's `/v1/systemone`, or is OpenJev a drop-in? Until answered,
   jev stays on llama-server.
5. Does our llama.cpp build already use the M5 neural accelerators? If not, upgrading llama.cpp might close much of the
   gap for less work — include a fresh llama.cpp build in the Phase 0 matrix.
6. Fallback if oMLX fails verification: LM Studio headless (`lms`) as the MLX host, or a small ai-mode-managed pool of
   `mlx_lm.server` processes (one per MLX role) behind the proxy.

## Sources

- mlx-lm server docs: https://github.com/ml-explore/mlx-lm/blob/main/mlx_lm/SERVER.md
- oMLX: https://github.com/jundot/omlx
- mlx-openai-server (multi-model via YAML, alternative): https://github.com/cubist38/mlx-openai-server
- MLX vs llama.cpp, short vs long context: https://vettedconsumer.com/mlx-vs-llama-cpp-on-a-mac-which-is-faster-and-when-the-answer-flips/ ,
  https://presenc.ai/research/mlx-vs-llama-cpp-throughput-benchmarks-2026
- M5 Max MLX numbers: https://aiproductivity.ai/blog/apple-m5-max-local-llm-guide/ , https://llmcheck.net/blog/apple-silicon-m5-max-local-ai-guide/
- Model repos: https://huggingface.co/mlx-community/Qwen3.8-27B-4bit , https://huggingface.co/mlx-community/qwen3.8-27b-mtp-4bit ,
  https://huggingface.co/mlx-community/gemma-4-26b-a4b-it-4bit , https://huggingface.co/mlx-community/gemma-4-26B-A4B-it-qat-4bit
- Gemma 4 tool-call issue on mlx_lm 0.31.2: https://github.com/ml-explore/mlx-lm/issues/1125
