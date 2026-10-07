# Model research notes

Selections verified against Hugging Face GGUF repos (existence, quant tags, mmproj,
documented context). Role fit combines **base model strengths** + **agent markdown**
(there are almost no true “PRD-only” GGUFs; specialists exist for security, Flux
prompts, and UI critique).

Hardware target: Mac Studio M5 Max, 128 GB unified memory.

## Orchestration (all profiles)

| Role | Model | API | Why |
| --- | --- | --- | --- |
| `chief` | Generative LLM (profile-specific) | `/v1/chat/completions` | Talks to the human; synthesizes specialist output |
| `jev` | `ggml-org/Kev-4B-GGUF:Q4_K_M` | **`/v1/systemone`** | System One decision model — closed-set role routing |

`jev` is **not** a chat model. Orchestrators must call System One with a closed
criterion set (see `agents/jev.md`). Requires a llama.cpp build that exposes
`/v1/systemone` (check with `ai-mode doctor`).

**`models-max` is profile-specific:** `learning-center` and `av-club` use **3** (chief + jev + one specialist); **dev-shop** uses **5** (warm fills all slots — see below).

## Sampling defaults (Qwen3 family)

From Qwen GGUF cards:

| Mode | temp | top_p | top_k | min_p | presence_penalty |
| --- | --- | --- | --- | --- | --- |
| Thinking | 0.6 | 0.95 | 20 | 0 | 1.5 |
| Non-thinking / tools | 0.7 | 0.8 | 20 | 0 | 1.5 |

Native Qwen3 dense context is **32 768** (extend with YaRN only when needed).
**Qwen3-Coder-Next** reports **262 144** native context in GGUF metadata.

**Superpowers + ai-mode (dev-shop):** The human stays on **`chief`** (131072 ctx).
Chief coordinates Superpowers flows and routes planning / large codebase reads to
**`architect`** (262144) via jev — the human should not need `/model` switches for
role routing. After a design spec is approved for review, run
**`spec-specialist-review`** before **`writing-plans`** (architect executes plans).
See `skills/spec-specialist-review/SKILL.md`.

### dev-shop resident memory (128 GB, `models-max = 5`)

Rough llama-server resident footprint (weights mmap + KV reserve at configured `ctx-size`).
Specialists and heavy roles use default **f16** KV; **chief** alone uses **`cache-type-k` /
`cache-type-v` = `q4_0`** (conversational frontend — quality work stays on specialists).
Use for “can these five slots coexist?” — not exact; leave headroom for macOS.

| Role | Weights (Q4) | KV @ ctx | ≈ resident |
| --- | --- | --- | --- |
| chief | ~8.4 GiB | ~5 GiB @ 131072 (q4_0 K/V) | ~13 GiB |
| jev | ~2.5 GiB | ~0.5 GiB @ 16384 | ~3 GiB |
| fast | ~2.5 GiB | ~0.5 GiB @ 16384 | ~3 GiB |
| architect | ~18 GiB | ~40 GiB @ 262144 | ~58 GiB |
| coder (before) | ~18 GiB | ~12 GiB @ 131072 (f16 K/V) | ~30 GiB |
| coder (after) | ~18 GiB | ~3 GiB @ 65536 (q8_0 K/V) | ~21 GiB |

**Before:** warm `chief,jev,architect,fast` ≈ **~77 GiB** — coder absent; loading coder evicted
a warm slot → chief + jev + architect + coder @ 131072 ≈ **~104 GiB**.

**After (superseded):** warm `chief,jev,architect,coder` ≈ **~95 GiB** reserved (~13 + 3 + 58 + 21) —
all four core roles co-resident with **`models-max = 4`**. **`fast`** loaded on demand (~3 GiB)
when Auto-mode hit `/model --fast fast`; with four full slots that often meant “Classifier stage 1 unavailable.”

**Current:** warm **`chief,jev,architect,coder,fast`** ≈ **~98 GiB** reserved (~13 + 3 + 58 + 21 + 3) —
orchestration, plan→execute stack, and Qwen Auto-mode classifiers co-resident with **`models-max = 5`** on 128 GB.

Architect stays **262144** (native 256k); coder **65536** (native, no YaRN) plus **q8_0** KV
was enough headroom — no architect ctx cut. If RSS is still tight in practice, next levers are
architect **131072** (~20 GiB KV save) or coder **32768** (~3 GiB more vs 65536).

Chief **131072** uses YaRN-extended ctx on Qwen3-14B (native 32k); chosen so chief can
hold long Superpowers threads and pasted specs without hitting the old 32k wall, without
promoting chief to 30B-A3B (would compete with architect/coder for slots).

## dev-shop

| Role | Model | Why | ctx |
| --- | --- | --- | --- |
| chief | `Qwen/Qwen3-14B-GGUF:Q4_K_M` | Sole user-facing POC (131k ctx, **q4_0 KV**); routes via jev; delegates planning / monorepo reads to architect | 131072 |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | System One role classifier | 16384 |
| fast | `Qwen/Qwen3-4B-GGUF:Q4_K_M` | Cheap instruct chat for classifiers, structured JSON, quick side-queries; **not** jev Stage A | 16384 |
| coder | `lmstudio-community/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M` | Agentic executor of architect's plans; **65536** native ctx + **q8_0** KV for five-slot warm; Q8 weights via ggml-org optional; `coder-xl` for huge jobs | 65536 |
| coder-xl | Local Qwen3-Coder-Next UD-Q8 | Heavy coder for rare under-specified / large jobs; already on disk | 131072 |
| product | `Qwen/Qwen3-14B-GGUF:Q4_K_M` | Specs/stories + spec review pass (YaRN ctx) | 65536 |
| research | `unsloth/Qwen3-30B-A3B-Thinking-2507-GGUF:Q4_K_M` | Thinking variant for spikes / tradeoffs | 65536 |
| docs | `unsloth/gemma-3-27b-it-GGUF:Q4_K_M` | Strong prose; 128k-class Gemma 3; auto mmproj unused for text | 65536 |
| qa | `unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M` | Code-aware test design (not Qwen2.5-Coder-7B) | 65536 |
| security | `deer-sec/CyberStag-Security-26B-A4B-V1-Q4_K_M-GGUF` | Security-tuned MoE GGUF | 65536 |
| design | `stefans71/frontend-design-expert-8b:Q4_K_M` | UI/frontend specialist (VL) + mmproj in repo | 16384 |
| ux | same as design (alias) or VL general — see ini | Screenshot critique trigger phrase in agent md | 16384 |
| architect | `bartowski/Qwen_Qwen3-30B-A3B-Thinking-2507-GGUF:Q4_K_M` | **Planning owner** — thinking model, native 256k ctx; alt `Qwen/Qwen3-32B-GGUF:Q4_K_M` (official, dense, YaRN>32k) | 262144 |

dev-shop **`warm = chief,jev,architect,coder,fast`** matches **`models-max = 5`**: orchestration,
plan + execute, and **`fast`** (Qwen Auto-mode Stage 1) are resident at startup. Other ini roles
(e.g. `product`, `qa`) load on demand and may evict a resident model until a slot frees.

See the **memory table** above (~98 GiB warm reserve) — live RSS is often lower until contexts
fill (e.g. architect ~42 GiB loaded vs ~58 GiB reserved). No official Qwen GGUF exists for the Thinking-2507 or Coder-30B-A3B
variants, so trusted non-Unsloth quantizers are used (bartowski / lmstudio-community /
ggml-org); the earlier Unsloth Qwen3-30B-A3B degeneration does not apply to these.

## learning-center

| Role | Model | Why | ctx |
| --- | --- | --- | --- |
| chief | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | User-facing learning lead (**q4_0 KV**) | 32768 |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | System One role classifier | 16384 |
| research-deep | `unsloth/DeepSeek-R1-Distill-Qwen-32B-GGUF:Q4_K_M` | Best-known dense reasoning distill for deep work | 65536 |
| research-qwen | `unsloth/Qwen3-30B-A3B-Thinking-2507-GGUF:Q4_K_M` | Newer Qwen3 thinking MoE alternative | 40960 |
| synthesizer | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | Outline / study guide structuring | 32768 |
| podcast-host | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | Conversational scripts (Mozilla doc→podcast used ~7B; 14B is better creative headroom) | 32768 |
| show-notes | `unsloth/Qwen3-4B-GGUF:Q4_K_M` | Fast titles / blurbs | 16384 |
| tutor | `unsloth/Qwen3-8B-GGUF:Q4_K_M` | Socratic Q&A over notes | 32768 |

TTS stays outside llama-server (Kokoro / Chatterbox / mlx-audio).

## av-club

| Role | Model | Why | ctx |
| --- | --- | --- | --- |
| chief | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | Creative lead talking to the human (**q4_0 KV**) | 32768 |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | System One role classifier | 16384 |
| director | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | Creative direction / shot lists | 32768 |
| prompt-crafter | `mradermacher/Qwen3-1.7B-Flux-Prompt-GGUF:Q4_K_M` | Flux-prompt fine-tune (not generic 8B) | 8192 |
| frame-review | `unsloth/Qwen3-VL-8B-Instruct-GGUF:Q4_K_M` | Strong local VLM for stills / OCR | 16384 |
| editor | `unsloth/Qwen3-8B-GGUF:Q4_K_M` | Edit notes / assembly language | 32768 |
| captioner | `unsloth/gemma-3-27b-it-GGUF:Q4_K_M` | Publish-ready copy | 32768 |
| tagger | `unsloth/Qwen3-1.7B-GGUF:Q4_K_M` | Cheap taxonomy / filenames | 8192 |

Image/video **generation** remains ComfyUI (+ your Flux GGUF), not these LLMs.
