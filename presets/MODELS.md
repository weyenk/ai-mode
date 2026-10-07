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

`models-max = 3` so chief + jev + one specialist can stay resident.

## Sampling defaults (Qwen3 family)

From Qwen GGUF cards:

| Mode | temp | top_p | top_k | min_p | presence_penalty |
| --- | --- | --- | --- | --- | --- |
| Thinking | 0.6 | 0.95 | 20 | 0 | 1.5 |
| Non-thinking / tools | 0.7 | 0.8 | 20 | 0 | 1.5 |

Native Qwen3 dense context is **32 768** (extend with YaRN only when needed).
**Qwen3-Coder-Next** reports **262 144** native context in GGUF metadata.

**Superpowers + ai-mode:** Architectural brainstorming and codebase exploration
should run on **`architect`** (262144), not **`chief`** (32768). After a
design spec is approved for review, run **`spec-specialist-review`** before
**`writing-plans`** on architect. See `skills/spec-specialist-review/SKILL.md`.

## dev-shop

| Role | Model | Why | ctx |
| --- | --- | --- | --- |
| chief | `Qwen/Qwen3-14B-GGUF:Q4_K_M` | Lightweight user-facing router (official); planning delegated to architect; **not** for monorepo exploration | 32768 |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | System One role classifier | 16384 |
| coder | `lmstudio-community/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M` | Fast agentic executor of architect's plans; Q8 via ggml-org for fidelity; local UD-Q8 kept as `coder-xl` | 131072 |
| coder-xl | Local Qwen3-Coder-Next UD-Q8 | Heavy coder for rare under-specified / large jobs; already on disk | 131072 |
| product | `Qwen/Qwen3-14B-GGUF:Q4_K_M` | Specs/stories + spec review pass (YaRN ctx) | 65536 |
| research | `unsloth/Qwen3-30B-A3B-Thinking-2507-GGUF:Q4_K_M` | Thinking variant for spikes / tradeoffs | 65536 |
| docs | `unsloth/gemma-3-27b-it-GGUF:Q4_K_M` | Strong prose; 128k-class Gemma 3; auto mmproj unused for text | 65536 |
| qa | `unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M` | Code-aware test design (not Qwen2.5-Coder-7B) | 65536 |
| security | `deer-sec/CyberStag-Security-26B-A4B-V1-Q4_K_M-GGUF` | Security-tuned MoE GGUF | 65536 |
| design | `stefans71/frontend-design-expert-8b:Q4_K_M` | UI/frontend specialist (VL) + mmproj in repo | 16384 |
| ux | same as design (alias) or VL general — see ini | Screenshot critique trigger phrase in agent md | 16384 |
| architect | `bartowski/Qwen_Qwen3-30B-A3B-Thinking-2507-GGUF:Q4_K_M` | **Planning owner** — thinking model, native 256k ctx; alt `Qwen/Qwen3-32B-GGUF:Q4_K_M` (official, dense, YaRN>32k) | 262144 |

With these footprints any resident trio (chief + jev + one specialist) is ~48 GB, so
`models-max = 4` can keep `architect` + `coder` co-resident for a plan→execute handoff
with no reload. No official Qwen GGUF exists for the Thinking-2507 or Coder-30B-A3B
variants, so trusted non-Unsloth quantizers are used (bartowski / lmstudio-community /
ggml-org); the earlier Unsloth Qwen3-30B-A3B degeneration does not apply to these.

## learning-center

| Role | Model | Why | ctx |
| --- | --- | --- | --- |
| chief | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | User-facing learning lead | 32768 |
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
| chief | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | Creative lead talking to the human | 32768 |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | System One role classifier | 16384 |
| director | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | Creative direction / shot lists | 32768 |
| prompt-crafter | `mradermacher/Qwen3-1.7B-Flux-Prompt-GGUF:Q4_K_M` | Flux-prompt fine-tune (not generic 8B) | 8192 |
| frame-review | `unsloth/Qwen3-VL-8B-Instruct-GGUF:Q4_K_M` | Strong local VLM for stills / OCR | 16384 |
| editor | `unsloth/Qwen3-8B-GGUF:Q4_K_M` | Edit notes / assembly language | 32768 |
| captioner | `unsloth/gemma-3-27b-it-GGUF:Q4_K_M` | Publish-ready copy | 32768 |
| tagger | `unsloth/Qwen3-1.7B-GGUF:Q4_K_M` | Cheap taxonomy / filenames | 8192 |

Image/video **generation** remains ComfyUI (+ your Flux GGUF), not these LLMs.
