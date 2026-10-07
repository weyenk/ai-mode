# ai-mode skills (Qwen / Superpowers)

| Skill | Purpose |
| --- | --- |
| [`spec-specialist-review`](spec-specialist-review/SKILL.md) | Multi-role review between brainstorming spec and `writing-plans` |
| [`testing-guidelines`](testing-guidelines/SKILL.md) | Load the vendored QA testing guidelines by layer before authoring/reviewing tests |

## Install for Qwen Code

Symlink into the Superpowers extension so the skills resolve:

```bash
ln -sfn "$(pwd)/skills/spec-specialist-review" \
  ~/.qwen/extensions/superpowers/skills/spec-specialist-review
ln -sfn "$(pwd)/skills/testing-guidelines" \
  ~/.qwen/extensions/superpowers/skills/testing-guidelines
```

Brainstorming is patched in place under `~/.qwen/extensions/superpowers/skills/brainstorming/` (original backed up as `SKILL.md.bak`). Re-apply after Superpowers extension upgrades if needed.
