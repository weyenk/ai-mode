# ai-mode skills (Qwen / Superpowers)

| Skill | Purpose |
| --- | --- |
| [`spec-specialist-review`](spec-specialist-review/SKILL.md) | Multi-role review between brainstorming spec and `writing-plans` |
| [`ai-mode-routing`](ai-mode-routing/SKILL.md) | Ad-hoc chief → specialist via Shell curl to ai-mode chat/completions |
| [`testing-guidelines`](testing-guidelines/SKILL.md) | Load the vendored QA testing guidelines by layer before authoring/reviewing tests |
| [`drafting-plans`](drafting-plans/SKILL.md) | Architect-authored plans: contract-first boundaries, dependency waves, exclusive file ownership, tests-first traceability (used instead of superpowers' writing-plans for ai-mode) |
| [`pr-stack`](pr-stack/SKILL.md) | Build, fix, restack and land a dependency-ordered stack of PRs with `gh stack`; local work is free, push/submit/merge need a human yes (the pipeline's stage 10 pushes and opens drafts per D3) |
| [`three-amigos`](three-amigos/SKILL.md) | Refine one story with `ai-mode amigos` into a Story Contract; a ready verdict is final |
| [`dev-pipeline`](dev-pipeline/SKILL.md) | Entry point to `ai-mode pipeline`: file a request, advance it through the ten stages, read the board, answer or unblock cards |
| [`viability`](viability/SKILL.md) | Stage 3: go/no-go and ROI from research, product and architect estimates |
| [`writing-specs`](writing-specs/SKILL.md) | Stage 4: architect writes the spec, six reviews, chief merges |
| [`story-breakdown`](story-breakdown/SKILL.md) | Stage 6: feature plan → story files and cards in stack order |
| [`stack-review`](stack-review/SKILL.md) | Stage 9: architect reviews each layer; fixes in the owning layer; restack |

## Install for Qwen Code

Symlink into Qwen Code's personal skills dir. These are independent of the Superpowers extension, which is currently disabled
(`qwen extensions disable superpowers`); the skills resolve as `ai-mode-routing`, not `superpowers:ai-mode-routing`.

```bash
mkdir -p ~/.qwen/skills
for s in spec-specialist-review testing-guidelines ai-mode-routing pr-stack drafting-plans three-amigos dev-pipeline viability writing-specs story-breakdown stack-review; do
  ln -sfn "$(pwd)/skills/$s" ~/.qwen/skills/$s
done
```

Routing rules that must apply in every session come from `ai-mode setup-qwen` (`~/.qwen/QWEN.md`), not from a skill.
`spec-specialist-review` still expects brainstorming/writing-plans from Superpowers; re-enable it (`qwen extensions enable superpowers`) to use that pipeline.
