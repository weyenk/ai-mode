# Model catalog

Per-profile inventory of what each **ai-mode** preset runs on **llama-server**. Authoritative
values are `presets/<profile>.ini` (models, ctx, sampling) and `presets/<profile>.mode`
(port, `models-max`, `warm`).

For memory estimates, quantizer choices, YaRN notes, and tuning levers, see
[`presets/.local/MODELS-notes.md`](.local/MODELS-notes.md) (gitignored; create locally if missing).

## Shared defaults

All profiles inherit section `[*]`:

| Option | Value |
| --- | --- |
| `jinja` | 1 |
| `flash-attn` | on |
| `n-gpu-layers` | 99 |
| `load-mode` | mmap |
| `top-k` | 20 |
| `min-p` | 0 |
| `presence-penalty` | 1.5 |

Roles below list **overrides** only when they differ from `[*]`. Empty temp/top-p means the role
does not set them (System One / jev). **KV** = `cache-type-k` / `cache-type-v` when set; otherwise
default (typically f16).

**`jev`** on every profile: `ggml-org/Kev-4B-GGUF:Q4_K_M`, ctx **16384**, API **`/v1/systemone`**
(not chat). Confirm with `ai-mode doctor`.

## dev-shop

| Setting | Value |
| --- | --- |
| Port | 8080 |
| Host | 127.0.0.1 |
| `models-max` | 5 |
| `warm` | chief, jev, architect, coder, fast |

| Role | Model | ctx | temp | top-p | KV | Other |
| --- | --- | --- | --- | --- | --- | --- |
| chief | `Qwen/Qwen3-14B-GGUF:Q4_K_M` | 131072 | 0.7 | 0.8 | q4_0 | `enable_thinking`: false |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | 16384 | — | — | default | System One only |
| fast | `Qwen/Qwen3-4B-GGUF:Q4_K_M` | 16384 | 0.7 | 0.8 | default | `enable_thinking`: false |
| coder | `lmstudio-community/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M` | 65536 | 0.7 | 0.8 | q8_0 | |
| coder-xl | local Qwen3-Coder-Next UD-Q8 (path in ini) | 131072 | 0.7 | 0.8 | default | |
| product | `Qwen/Qwen3-14B-GGUF:Q4_K_M` | 65536 | 0.5 | 0.8 | default | |
| research | `unsloth/Qwen3-30B-A3B-Thinking-2507-GGUF:Q4_K_M` | 65536 | 0.6 | 0.95 | default | thinking on |
| docs | `unsloth/gemma-3-27b-it-GGUF:Q4_K_M` | 65536 | 0.6 | 0.9 | default | `presence-penalty`: 0.5 |
| qa | `unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M` | 65536 | 0.3 | 0.8 | default | |
| security | `deer-sec/CyberStag-Security-26B-A4B-V1-Q4_K_M-GGUF` | 65536 | 0.2 | 0.85 | default | `presence-penalty`: 0.5 |
| design | `stefans71/frontend-design-expert-8b:Q4_K_M` | 16384 | 0.4 | 0.9 | default | VL + mmproj; `presence-penalty`: 0; `enable_thinking`: false |
| ux | same as design | 16384 | 0.4 | 0.9 | default | alias role |
| architect | `bartowski/Qwen_Qwen3-30B-A3B-Thinking-2507-GGUF:Q4_K_M` | 262144 | 0.6 | 0.95 | default | thinking on; alt `Qwen/Qwen3-32B-GGUF:Q4_K_M` @ 65536 |

Non-warm roles load on demand and may evict a resident model until a slot frees.

## learning-center

| Setting | Value |
| --- | --- |
| Port | 8081 |
| Host | 127.0.0.1 |
| `models-max` | 3 |
| `warm` | chief, jev |

| Role | Model | ctx | temp | top-p | KV | Other |
| --- | --- | --- | --- | --- | --- | --- |
| chief | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | 32768 | 0.7 | 0.8 | q4_0 | |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | 16384 | — | — | default | System One only |
| research-deep | `unsloth/DeepSeek-R1-Distill-Qwen-32B-GGUF:Q4_K_M` | 65536 | 0.6 | 0.95 | default | |
| research-qwen | `unsloth/Qwen3-30B-A3B-Thinking-2507-GGUF:Q4_K_M` | 40960 | 0.6 | 0.95 | default | thinking on |
| synthesizer | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | 32768 | 0.5 | 0.8 | default | |
| podcast-host | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | 32768 | 0.9 | 0.95 | default | `top-k`: 40; `presence-penalty`: 1.2 |
| show-notes | `unsloth/Qwen3-4B-GGUF:Q4_K_M` | 16384 | 0.5 | 0.8 | default | |
| tutor | `unsloth/Qwen3-8B-GGUF:Q4_K_M` | 32768 | 0.6 | 0.8 | default | |

Podcast **audio** is outside llama-server (TTS). See notes file.

## av-club

| Setting | Value |
| --- | --- |
| Port | 8082 |
| Host | 127.0.0.1 |
| `models-max` | 3 |
| `warm` | chief, jev |

| Role | Model | ctx | temp | top-p | KV | Other |
| --- | --- | --- | --- | --- | --- | --- |
| chief | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | 32768 | 0.85 | 0.95 | q4_0 | |
| jev | `ggml-org/Kev-4B-GGUF:Q4_K_M` | 16384 | — | — | default | System One only |
| director | `unsloth/Qwen3-14B-GGUF:Q4_K_M` | 32768 | 0.85 | 0.95 | default | |
| prompt-crafter | `mradermacher/Qwen3-1.7B-Flux-Prompt-GGUF:Q4_K_M` | 8192 | 0.7 | 0.9 | default | `presence-penalty`: 0.5 |
| frame-review | `unsloth/Qwen3-VL-8B-Instruct-GGUF:Q4_K_M` | 16384 | 0.3 | 0.8 | default | VL + mmproj; `presence-penalty`: 0.5 |
| editor | `unsloth/Qwen3-8B-GGUF:Q4_K_M` | 32768 | 0.5 | 0.8 | default | |
| captioner | `unsloth/gemma-3-27b-it-GGUF:Q4_K_M` | 32768 | 0.6 | 0.9 | default | `presence-penalty`: 0.5 |
| tagger | `unsloth/Qwen3-1.7B-GGUF:Q4_K_M` | 8192 | 0.2 | 0.8 | default | `presence-penalty`: 0 |

Image/video **generation** is ComfyUI (+ your Flux GGUF), not these LLMs. See notes file.
