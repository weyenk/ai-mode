# ai-mode

System-wide CLI to switch **llama-server** profile presets and load per-role
**agent** system prompts.

| Profile | Port | Focus |
| --- | --- | --- |
| `dev-shop` | 8080 | Chief + Kev-4B jev router + coding/product/QA/UX specialists |
| `learning-center` | 8081 | Chief + jev → research / notes / podcast script |
| `av-club` | 8082 | Chief + jev → photo/video language (ComfyUI renders pixels) |

Orchestration: **`chief`** (chat) talks to you; **`jev`** (`ggml-org/Kev-4B-GGUF`) classifies via `/v1/systemone`; specialists execute.

Model choices, context sizes, and sampling are documented in [`presets/MODELS.md`](presets/MODELS.md).  
Role system prompts live in [`presets/agents/`](presets/agents/).

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
eval "$(ai-mode env)"          # OPENAI_BASE_URL for this shell

# Example chat with role prompt
SYS="$(ai-mode prompt qa)"
curl -s "$(ai-mode url)/chat/completions" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg s "$SYS" '{
    model:"qa",
    messages:[
      {role:"system",content:$s},
      {role:"user",content:"Write edge cases for login rate limiting."}
    ]
  }')"
```

Also: `stop`, `restart`, `logs [-f]`, `url`, `models`, `init <name> --with-ini`.

## Adding a profile later

1. `ai-mode init my-lab --with-ini --port 8083`
2. Edit `presets/my-lab.ini` (models + ctx/sampling)
3. Add `presets/agents/<role>.md` for each `[section]`
4. Note rationale in `presets/MODELS.md`

## Notes

- First `hf = …` request downloads into `~/.cache/llama.cpp` (can be large).
- VL roles auto-pull `mmproj` when the HF repo includes one.
- For `design` / `ux` screenshots, user text should include: `Critique this UI design.`
- Podcast **audio** and **image/video generation** are outside llama-server (TTS / ComfyUI).
