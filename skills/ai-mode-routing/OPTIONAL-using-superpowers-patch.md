# Optional: upstream using-superpowers

If you reset `~/.qwen/extensions/superpowers/skills/using-superpowers/` from upstream, re-apply these two lines (already applied in the local Qwen copy as of this skill):

**Skill Priority** — add bullet:

```markdown
- "Ask the product agent …" / consult an ai-mode specialist → **superpowers:ai-mode-routing** (HTTP to `$(ai-mode url)/chat/completions` or `/model <role>`) — **not** another using-superpowers pass or Qwen `Task`/`Agent` as the specialist.
```

**Platform Adaptation** — add bullet:

```markdown
- Qwen Code + local **ai-mode**: **`/superpowers:ai-mode-routing`** when the human names product, security, or another preset role for an ad-hoc ask (see repo `skills/ai-mode-routing/SKILL.md`).
```

Chief + `ai-mode-routing` alone are enough for discovery via skill description; this patch only helps `using-superpowers` priority routing.
