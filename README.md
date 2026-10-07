# ai-mode

System-wide CLI to switch **llama-server** profile presets (`dev-shop`, `learning-center`, `av-club`, …).

## Install

```bash
./install.sh
# or:
ln -sf "$(pwd)/bin/ai-mode" ~/.local/bin/ai-mode
export AI_MODE_PRESETS="$(pwd)/presets"
```

## Usage

```bash
ai-mode list
ai-mode use dev-shop
ai-mode which
ai-mode doctor
eval "$(ai-mode env)"
ai-mode stop
```

Profiles are any `presets/<name>.ini` plus optional `presets/<name>.mode` (port, description). See `presets/` for the bundled studios.
