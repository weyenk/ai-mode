#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$HOME/.local/bin" "$HOME/.config/ai-mode" "$HOME/.zsh/completions"

ln -sfn "$ROOT/bin/ai-mode" "$HOME/.local/bin/ai-mode"
if [[ -f "$ROOT/completions/_ai-mode" ]]; then
  ln -sfn "$ROOT/completions/_ai-mode" "$HOME/.zsh/completions/_ai-mode"
fi

cat > "$HOME/.config/ai-mode/config" <<EOF
AI_MODE_PRESETS=$ROOT/presets
EOF

echo "Installed ai-mode → $HOME/.local/bin/ai-mode"
echo "Presets → $ROOT/presets (set in ~/.config/ai-mode/config)"
echo "Ensure ~/.local/bin is on PATH, then: ai-mode doctor"
