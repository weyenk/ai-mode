# Model catalog

Per-profile inventory for **ai-mode** presets on **llama-server**. Authoritative values:
`presets/<profile>.ini` (models, ctx, sampling) and `presets/<profile>.mode` (port, `models-max`, `warm`).

Rationale, memory math, sampling defaults, and tuning levers:
[`presets/.local/MODELS-notes.md`](.local/MODELS-notes.md) (gitignored; create locally if missing).

## Orchestration (all profiles)

| Role | Model | API | Why |
| --- | --- | --- | --- |
| `chief` | Generative LLM (profile-specific) | `/v1/chat/completions` | Talks to the human; synthesizes specialist output |
| `jev` | `ggml-org/Kev-4B-GGUF:Q4_K_M` | **`/v1/systemone`** | System One decision model — closed-set role routing |

`jev` is **not** a chat model. Orchestrators must call System One with a closed criterion set (see `agents/jev.md`). Requires a llama.cpp build that exposes `/v1/systemone` (check with `ai-mode doctor`).

**`models-max` is profile-specific:** `learning-center` and `av-club` use **3** (chief + jev + one specialist); **dev-shop** uses **5** (warm fills all slots — see below).

## dev-shop

| Role | Model | Why | ctx | ≈ GiB |
| --- | --- | --- | --- | --- |
| chief | `lmstudio-community/Qwen3-30B-A3B-Instruct-2507-GGUF:Q4_K_M` | Sole user-facing POC (131k ctx, native 262k, **q4_0 KV**, non-thinking); routes via jev; delegates planning / monorepo reads to architect. **Test swap** from Qwen3-14B (capped at 40960 by llama-server) | 131k | ~22 (est.) |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | System One role classifier | 16k | ~3 |
| fast | `Qwen/Qwen3-4B-GGUF:Q4_K_M` | Cheap instruct chat for classifiers, structured JSON, quick side-queries; **not** jev Stage A | 16k | ~3 |
| coder | `lmstudio-community/Devstral-Small-2-24B-Instruct-2512-GGUF:Q4_K_M` | Agentic executor of architect's plans; Mistral (non-Qwen) so it is adversarial to architect; **64k** ctx + **q8_0** KV (native 256k); `coder-xl` for huge jobs | 64k | ~20 (est.) |
| coder-xl | Local Qwen3-Coder-Next UD-Q8 | Heavy coder for rare under-specified / large jobs; already on disk | 131k | ~40 |
| product | `Qwen/Qwen3-14B-GGUF:Q4_K_M` | Specs/stories + spec review pass (YaRN ctx) | 64k | ~11 |
| research | `unsloth/Qwen3-30B-A3B-Thinking-2507-GGUF:Q4_K_M` | Thinking variant for spikes / tradeoffs | 64k | ~25 |
| docs | `unsloth/gemma-3-27b-it-GGUF:Q4_K_M` | Strong prose; 128k-class Gemma 3; auto mmproj unused for text | 64k | ~26 |
| qa | `unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M` | Code-aware test design (not Qwen2.5-Coder-7B) | 64k | ~21 |
| security | `deer-sec/CyberStag-Security-26B-A4B-V1-Q4_K_M-GGUF` | Security-tuned MoE GGUF | 64k | ~28 |
| design | `stefans71/frontend-design-expert-8b:Q4_K_M` | UI/frontend specialist (VL) + mmproj in repo | 16k | ~7 |
| ux | same as design (alias) or VL general — see ini | Screenshot critique trigger phrase in agent md | 16k | ~7 |
| architect | `bartowski/Qwen_Qwen3-30B-A3B-Thinking-2507-GGUF:Q4_K_M` | **Planning owner** — thinking model, native 256k ctx; alt `Qwen/Qwen3-32B-GGUF:Q4_K_M` (official, dense, YaRN>32k) | 256k | ~58 |

dev-shop **`warm = chief,jev,architect,coder,fast`** matches **`models-max = 5`**: orchestration, plan + execute, and **`fast`** (Qwen Auto-mode Stage 1) are resident at startup (~**98 GiB** reserved — see notes). Other ini roles load on demand and may evict a resident model until a slot frees.

## learning-center

| Role | Model | Why | ctx | ≈ GiB |
| --- | --- | --- | --- | --- |
| chief | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | User-facing learning lead (**q4_0 KV**) | 32k | ~10 |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | System One role classifier | 16k | ~3 |
| research-deep | `unsloth/DeepSeek-R1-Distill-Qwen-32B-GGUF:Q4_K_M` | Best-known dense reasoning distill for deep work | 64k | ~30 |
| research-qwen | `unsloth/Qwen3-30B-A3B-Thinking-2507-GGUF:Q4_K_M` | Newer Qwen3 thinking MoE alternative | 40k | ~22 |
| synthesizer | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | Outline / study guide structuring | 32k | ~10 |
| podcast-host | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | Conversational scripts (Mozilla doc→podcast used ~7B; 14B is better creative headroom) | 32k | ~10 |
| show-notes | `unsloth/Qwen3-4B-GGUF:Q4_K_M` | Fast titles / blurbs | 16k | ~3 |
| tutor | `unsloth/Qwen3-8B-GGUF:Q4_K_M` | Socratic Q&A over notes | 32k | ~6 |

TTS stays outside llama-server (Kokoro / Chatterbox / mlx-audio).

## av-club

| Role | Model | Why | ctx | ≈ GiB |
| --- | --- | --- | --- | --- |
| chief | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | Creative lead talking to the human (**q4_0 KV**) | 32k | ~10 |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | System One role classifier | 16k | ~3 |
| director | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | Creative direction / shot lists | 32k | ~10 |
| prompt-crafter | `mradermacher/Qwen3-1.7B-Flux-Prompt-GGUF:Q4_K_M` | Flux-prompt fine-tune (not generic 8B) | 8k | ~2 |
| frame-review | `unsloth/Qwen3-VL-8B-Instruct-GGUF:Q4_K_M` | Strong local VLM for stills / OCR | 16k | ~8 |
| editor | `unsloth/Qwen3-8B-GGUF:Q4_K_M` | Edit notes / assembly language | 32k | ~6 |
| captioner | `unsloth/gemma-3-27b-it-GGUF:Q4_K_M` | Publish-ready copy | 32k | ~22 |
| tagger | `unsloth/Qwen3-1.7B-GGUF:Q4_K_M` | Cheap taxonomy / filenames | 8k | ~2 |

Image/video **generation** remains ComfyUI (+ your Flux GGUF), not these LLMs.
