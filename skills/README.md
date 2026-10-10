# ai-mode skills (Qwen / Superpowers)

| Skill | Purpose |
| --- | --- |
| [`spec-specialist-review`](spec-specialist-review/SKILL.md) | Multi-role review between brainstorming spec and `writing-plans` |
| [`ai-mode-routing`](ai-mode-routing/SKILL.md) | Ad-hoc chief → specialist via Shell curl to ai-mode chat/completions |
| [`testing-guidelines`](testing-guidelines/SKILL.md) | Load the vendored QA testing guidelines by layer before authoring/reviewing tests |
| [`drafting-plans`](drafting-plans/SKILL.md) | Architect-authored plans: contract-first boundaries, dependency waves, exclusive file ownership, tests-first traceability (used instead of superpowers' writing-plans for ai-mode) |
| [`pr-stack`](pr-stack/SKILL.md) | Build, fix, restack and land a dependency-ordered stack of PRs with `gh stack`; local work is free, push/submit/merge need a human yes |

## Install for Qwen Code

Symlink into Qwen Code's personal skills dir. These are independent of the Superpowers extension, which is currently disabled
(`qwen extensions disable superpowers`); the skills resolve as `ai-mode-routing`, not `superpowers:ai-mode-routing`.

```bash
mkdir -p ~/.qwen/skills
for s in spec-specialist-review testing-guidelines ai-mode-routing pr-stack drafting-plans; do
  ln -sfn "$(pwd)/skills/$s" ~/.qwen/skills/$s
done
```

Routing rules that must apply in every session come from `ai-mode setup-qwen` (`~/.qwen/QWEN.md`), not from a skill.
`spec-specialist-review` still expects brainstorming/writing-plans from Superpowers; re-enable it (`qwen extensions enable superpowers`) to use that pipeline.
