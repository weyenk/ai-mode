# ai-mode skills (Qwen / Superpowers)

| Skill | Purpose |
| --- | --- |
| [`spec-specialist-review`](spec-specialist-review/SKILL.md) | Multi-role review between brainstorming spec and `writing-plans` |

## Install for Qwen Code

Symlink into the Superpowers extension so `/superpowers:spec-specialist-review` resolves:

```bash
ln -sfn "$(pwd)/skills/spec-specialist-review" \
  ~/.qwen/extensions/superpowers/skills/spec-specialist-review
```

Brainstorming is patched in place under `~/.qwen/extensions/superpowers/skills/brainstorming/` (original backed up as `SKILL.md.bak`). Re-apply after Superpowers extension upgrades if needed.
