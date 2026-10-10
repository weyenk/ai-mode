# Plan: fork superpowers as `ai-mode-skills`

Status: Phase 1 in progress. Source: `~/.qwen/extensions/superpowers` (obra/superpowers v6.4.2, MIT).
`ai-mode-skills` is a temporary name.

## Phase 1: relocate and rename (mechanical)
1. Create `ai-mode-skills/` with `skills/`, `qwen-extension.json` (name `ai-mode-skills`), `GEMINI.md` and the session-start hook.
   Leave out Codex/Cursor/Devin/Kimi/Hermes/Pi/OpenCode packaging, CI scripts and upstream tests.
2. Keep `LICENSE` (MIT, upstream copyright) and add `NOTICE` crediting upstream. MIT requires both.
3. Rename `using-superpowers` -> `using-ai-mode-skills`, `diagnosing-superpowers` -> `diagnosing-ai-mode-skills`,
   `superpowers:<skill>` -> `ai-mode-skills:<skill>`, and all remaining mentions, including `presets/agents/chief.md`,
   README, AGENTS.md and the repo's existing skills.
4. Move `ai-mode-routing`, `spec-specialist-review`, `testing-guidelines` into the new layout; drop the symlink scheme.
5. Install: back up, then replace `~/.qwen/extensions/superpowers` with a symlink to `ai-mode-skills/`. **Needs explicit OK.**
   Qwen caches extension metadata in `~/.qwen/extension-store`; a reinstall or cache clear may be needed.
6. `ai-mode doctor` check: extension link and skill list intact; flag a leftover old install.

## Phase 2: rework skills (one commit each)
1. `brainstorming`: call `ai-mode team` early, drop stale 32k/131k text, chief delegates instead of exploring.
2. `using-ai-mode-skills`: shorten the "1% chance" framing for a 30B model.
3. `writing-plans`, `subagent-driven-development`, `executing-plans`: replace Task/Agent subagents with `ai-mode ask` to architect/coder/qa.
4. `test-driven-development`, `requesting-code-review`, `verification-before-completion`: wire to the qa and architect -> coder -> qa loop.
5. Cut unused material (visual companion, cross-harness text); update `skills/README.md`.

## Phase 3: verify
- Re-run the original web-version brainstorming prompt in Qwen; pass = `ai-mode team` runs and chief makes at most 3 read-only calls.
- Add a structure test: valid frontmatter per skill, no leftover "superpowers" strings.
