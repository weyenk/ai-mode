---
name: ai-mode-routing
description: "Consult ai-mode specialists (product, security, research, ux, design, qa, docs, architect, coder) from chief. Use when the human says ask the product agent, route to specialist, talk to security, ai-mode role, names any dev-shop specialist, or wants the whole team / everyone's input / to figure something out together (open-ended, early exploration) — not for spec-specialist-review (use that skill)."
---

# ai-mode routing

Consult a real llama-server role from chief. **Do exactly this, in this turn:**

```bash
ai-mode ask <role> "<question>" --caller chief --max-tokens 2048
```

Then summarize stdout for the human, naming the role ("**architect** found …").

That is the whole skill. Run the command **now** — do not ask the human anything first, and do not reload this skill.

<HARD-GATE>
- The specialist has only answered if `ai-mode ask` exited 0 with non-empty stdout. Otherwise say it failed (server down? wrong role? `ai-mode doctor`) and stop. Never invent or paraphrase an answer you did not receive.
- Do **not** use `Agent`, `Task`, `AskUserQuestion`, `Skill(using-superpowers)`, or `/model` as a substitute. Role names are llama-server model ids, not Qwen subagent types.
- Skill loaded, "Using ai-mode-routing…" announced, or "already loaded in context" does **not** count as consulting anyone.
</HARD-GATE>

## Whole team / open-ended asks

If the human wants the **whole team**, "everyone's view", or an exploration you can't answer yourself ("what's possible?", "figure this out together"), run **one** command instead of several asks:

```bash
ai-mode team "<the human's ask + repo path>" --caller chief
```

It calls architect first (repo mapping), then product, research, ux, security, and prints a single report (also saved under `~/.local/state/ai-mode/team/`). `--roles a,b,c` narrows or reorders the panel. Summarize by role; a `FAILED` section means that role did not answer. Do **not** explore the repo on chief first: at most 3 read-only calls before delegating.

## Building the question

For a single named role, the human names it and **you write the question.** If the request is broad ("review this project for flaws"), do not ask what they mean — fill in a sensible default and send it. Specialists have large contexts but cannot read files by themselves (they only see the prompt); chief must not explore either. Hand them the material with `--context`.

Examples:

```bash
# "ask the architect to review this project for flaws"
ai-mode ask architect "Review the repo at $PWD for design and correctness flaws: architecture, error handling, concurrency, config/preset consistency, missing tests. Rank findings by severity with file paths." --caller chief --max-tokens 4096

# "what would security think of storing tokens in settings.json?"
ai-mode ask security "Threat-model storing API tokens in ~/.qwen/settings.json for a local-only llama-server setup. Top risks and mitigations." --caller chief

# long context: pipe it
cat docs/superpowers/specs/foo-design.md | ai-mode ask product "Review this spec for scope and missing user stories." --caller chief
```

Rules of thumb:
- Include the human's words, the repo path or file paths, and the output shape you want (ranked list, yes/no + reasons, …).
- Roles cannot open paths. Ground them with `--context <file>` (full contents) and `--context <dir>` (file listing), repeatable; a context over the limit is an error, so pick the files that matter.
- Use `--max-tokens 4096` or more for reviews and plans.
- One role per call. For several roles, run several calls and synthesize.

## Only if the role is unclear

If the human did **not** name a role ("who should look at this?"):

```bash
ai-mode classify role "<clean task statement>" --caller chief
```

Read `choice` and `decision`. `clear` → `ai-mode ask <choice> …`. `ambiguous` / `low-confidence` → ask the human **one** question, then re-run. Pass the printed `trace=` / `span=` to the next call as `--trace <id> --parent <span>`.

If `choice` is `qa`: also run `ai-mode classify layer "<task>" --caller chief`, read the printed `load:` guideline path(s), then ask `qa`.

`coder-xl` is never routed by jev; use it only when the human asks for the heavy coder.

## Fallback

Only after `ai-mode ask` has failed and you told the human: `/model <role>`, ask, then `/model chief` before the next turn.

## Related

- Fixed six-pass spec pipeline → `skills/spec-specialist-review/SKILL.md`
- Chief system prompt → `presets/agents/chief.md` (**Ad-hoc specialist routing**)
- Jev contract → `presets/agents/jev.md`; QA layers → `skills/testing-guidelines/SKILL.md`
