# Plan: MLX backend (measure first)

Status: **Phase 0 not started.** Spec: `docs/superpowers/specs/2026-10-11-mlx-backend-design.md`.
This is the step-level roadmap. Before coding Phase 0b, architect turns it into a `drafting-plans` plan (task graph,
packets) and runs `ai-mode plan lint` and `ai-mode plan verify` on it. Phases 1 and 2 are planned in detail only after the Phase 0 numbers are in.

Who does what: **Mac** = run on the Mac Studio (needs Apple Silicon, the models, the GPU). **Cloud** = can be built and
unit-tested anywhere (Linux container, `go test` with `httptest` fakes; no models needed).

## Phase 0a: prerequisites (Mac, ~1-2 h, mostly downloads)

1. Install pinned tools, each in its own environment:
   - `mlx-lm` (reference server): `uv tool install "mlx-lm==<pin>"`, then confirm `mlx_lm.server --help` works.
   - oMLX via Homebrew (`omlx serve`), pinned, **auto-update off**, started with `--host 127.0.0.1` only.
   - A fresh llama.cpp build alongside the current one (spec Open #5). Record both `llama-server --version` strings.
2. Download the candidate MLX weights with `hf download`, pinned to a revision:
   - `mlx-community/Qwen3-30B-A3B-Instruct-2507-4bit` (chief), `mlx-community/Qwen3.8-27B-4bit` and
     `mlx-community/Qwen3.8-27B-MTP-4bit` (coder/qa), `mlx-community/gemma-4-26b-a4b-it-4bit` (architect),
     `mlx-community/Qwen3-4B-4bit` (fast). Check each repo exists first; ~65 GB total.
   - Record each repo + revision hash in `presets/.local/MODELS-notes.md`.
3. Check free disk: GGUF and MLX copies of the same model coexist during the trial.

**Done when:** each server answers `GET /v1/models` and one chat request for each candidate model.

## Phase 0b: `ai-mode bench` (Cloud build, Mac run)

Each step is test-first. Tests use recorded SSE fixtures and an `httptest` server, so none of them need a model.

1. **Contract.** `bench.go` types: `BenchCase{Role, Ctx, Gen, Run}`, `BenchResult{TTFT, PrefillTPS, DecodeTPS,
   PromptTokens, CompletionTokens, PeakMemBytes, Source}`; interfaces `chatStreamer` and `memSampler`; recorded stream
   fixtures for llama-server (has a `timings` block), `mlx_lm.server` and oMLX.
2. **Streaming client.** POST `/v1/chat/completions` with `stream: true` and
   `stream_options: {include_usage: true}`. TTFT is the time to the first content *or* reasoning delta. Decode tok/s is
   `completion_tokens / (t_last − t_first)`. When the server reports `timings` (llama-server), keep those as well and
   label each number with its source.
3. **Prompt builder.** Builds a deterministic prompt of N tokens from real repo text (the `--context` machinery), so the
   prompts look like ours. A random nonce goes **first** so prefix caches cannot fake prefill. The reported size is
   `usage.prompt_tokens`, not the estimate. Fixed sampling: `temperature 0`, thinking off via
   per-request `chat_template_kwargs`, `max_tokens = --gen`, and a "write at length" instruction. Runs whose
   `completion_tokens` come in under 80% of `--gen` are flagged.
4. **Memory sampler.** Peak footprint of a pid, sampled every 500 ms during each run, using macOS `footprint` with `ps -o rss`
   as a fallback. The command runner is injected so tests can fake it. The pid comes from ai-mode state, or from `--pid` for servers started by hand.
5. **Report + compare.** Prints a table, saves `bench/<id>.json` in the state dir, and emits `bench_run` events so `stats`
   sees them. `ai-mode bench compare A.json B.json` prints per-role deltas and the spec's go/no-go per role (decode
   ≥ +20%, TTFT not worse, memory within budget). Quality is reported as "pending" until step 0c fills it in.
6. **Integration.** Register in `cli.go`, and update `completions/_ai-mode`, the README section and the AGENTS.md CLI must-knows.
   Flags: `ai-mode bench <role>... [--base-url URL] [--pid N] [--ctx 4k,32k,128k] [--gen 512] [--runs 3] [--json] [--out FILE]`.

**Done when:** `make test` is green and the command runs end to end against the live dev-shop profile.

## Phase 0c: quality re-check (Cloud build if scripts are shared; Mac run)

1. The 2026-10 bake-off harness lives in gitignored `presets/.local/`. **Needs from you:** the scripts and cases
   (or a description of them). Port the minimum needed into `evals/bakeoff/`: the architect spec → coder → QA-mutant loop and
   its scoring. Each harness gets a `--base-url` per role so GGUF and MLX builds can be mixed.
2. jev stays on llama-server, so `make eval` is unchanged. An OpenJev trial is out of scope for Phase 0.

**Done when:** the harness reproduces the recorded GGUF scores (coder/architect 6/6 cells, qa 11/11 mutants) within
run-to-run noise. That proves the harness before any MLX number is trusted.

## Phase 0d: oMLX spike (Mac, ~1 h, no code)

Answer spec Open #1-4 with curl against a hand-started `omlx serve`, and write the answers into the spec:
1. Does `/v1/models` report load status?
2. Can per-model config (alias, sampling, kwargs, pin, TTL) be set by file or API with no app running?
3. Are KV quantization and a per-model context cap supported?
4. Does `thinking_budget` stop a long think and still produce an answer (try it on the Gemma prompt that ran away in the
   2026-10-09 long-context test)?

## Phase 0e: run and decide (Mac, ~2-4 h unattended)

1. `ai-mode use dev-shop`, then
   `ai-mode bench chief coder architect fast --ctx 4k,32k,128k --runs 3 --out gguf.json`. Run architect again at `--ctx 250k`
   (use `--runs 2`; one 250k prefill takes minutes).
2. Repeat against `mlx_lm.server` (one model at a time, `--base-url`, `--pid`) and against oMLX. Repeat once on the fresh
   llama.cpp build.
3. Rerun the 0c quality harness for coder, qa and architect on the MLX builds.
4. Run `ai-mode bench compare` for each pair. Write the numbers into `MODELS-notes.md` and a decision table into the spec.

**Exit:** per role, one of *move to MLX*, *stay on GGUF*, or *upgrade llama.cpp instead*. If nothing moves, stop here.
The bench command stays useful for future model swaps.

## Phase 1: backend abstraction (only if something moves)

1. Contract: a `backend` interface (`Start`, `Stop`, `Catalog`, `Load`, `EffectiveCtx`, `LogLines`, `Doctor`) and a fake.
2. `llamaBackend` wraps today's code. Existing tests must pass unchanged; this is a pure refactor.
3. `.mode` gets `backend =` and the ini gets the `mlx =` key. Add parsing and validation, plus the AGENTS.md preset-rule exception.
4. `omlxBackend`: generate oMLX config from the ini into the state dir, start/stop, catalog, warm via pins. Built
   against what Phase 0d found.
5. `doctor` and `logs` go through the backend. `doctor` warns on ini keys that have no MLX equivalent.

## Phase 2: hybrid profile

1. Proxy routes by the request's `model` and by path (`/v1/systemone` → llama), and merges `/v1/models`.
2. `backend = hybrid` starts both servers on internal ports. `which` shows the backend for each role, and `doctor` adds up memory across both.
3. Generate the API key, write it into the Qwen Code entries with `setup-qwen`, and keep `/admin` off the LAN.
4. Move the dev-shop roles that won in Phase 0e. Update `MODELS.md`.
