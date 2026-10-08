#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$HOME/.local/bin" "$HOME/.config/ai-mode" "$HOME/.zsh/completions"

command -v go >/dev/null || { echo "Go not found: brew install go" >&2; exit 1; }
(cd "$ROOT" && make build)

ln -sfn "$ROOT/dist/ai-mode" "$HOME/.local/bin/ai-mode"
if [[ -f "$ROOT/completions/_ai-mode" ]]; then
  ln -sfn "$ROOT/completions/_ai-mode" "$HOME/.zsh/completions/_ai-mode"
fi

cat > "$HOME/.config/ai-mode/config" <<EOT
AI_MODE_PRESETS=$ROOT/presets
EOT

echo "Installed ai-mode → $HOME/.local/bin/ai-mode"
echo "Presets → $ROOT/presets (set in ~/.config/ai-mode/config)"
case ":$PATH:" in
  *":$HOME/.local/bin:"*)
    echo "Next: ai-mode doctor"
    ;;
  *)
    case "$(basename "${SHELL:-zsh}")" in
      fish) fix='fish_add_path "$HOME/.local/bin"'; reload='' ;;
      bash) rc="$HOME/.bashrc"; [[ "$(uname)" == Darwin && ! -f "$rc" ]] && rc="$HOME/.bash_profile"
            fix="echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> $rc"; reload="source $rc" ;;
      *)    fix="echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshenv"; reload='source ~/.zshenv' ;;
    esac
    echo
    echo "~/.local/bin is not on your PATH. Run:"
    echo
    echo "  ${fix}${reload:+ && $reload} && ai-mode doctor"
    ;;
esac
