---
name: testing-guidelines
description: "Load the ai-mode QA testing guidelines when designing or reviewing tests. Two-stage jev routing picks the layer; qa loads guides and authors. Covers unit, functional, integration, component, contract, e2e, visual regression, mutation, and AI-skill/eval layers."
---

# Testing guidelines

House QA standards for the **qa** role, vendored under
`presets/agents/qa/guidelines/`. **jev** (System One) chooses the test layer; **qa**
loads the matching guide(s) and authors cases.

## When to use

Trigger when a task involves designing, writing, or reviewing tests — test plans, edge
cases, regression checklists, or test-quality review.

## Two-stage flow

1. **Stage A — role** (`jev`, question `role`): route to `qa` when the task is test
   design, cases, or regression strategy. See `presets/agents/jev.md`.
2. **Stage B — test layer** (`jev`, question `test_layer`): run when Stage A chose `qa`
   or the user explicitly asked for test strategy. Never combine role and layer in one
   System One request.
3. **Load guide(s)** from `presets/agents/qa/guidelines/<layer>.md` (map in `jev.md`).
   At most 2–3 files; follow jev confidence policy (one guide, or top two if close).
4. **Author** as setup → action → expected; cite each guide's *Quick Reference Checklist*.

If the harness already passed `test_layer`, skip Stage B and load guides directly.

### Running the classification

```bash
ai-mode classify layer "<user task + brief context>" --caller chief
```

Prints jev's `choice`, `confidence`, a `decision` (`clear` / `ambiguous` / `low-confidence`),
the ranking, and the guideline file(s) to load (`load:`): one when clear, the top two when
ambiguous. Add `--json` for machine-readable output, and `--trace <id> --parent <span>` to
link it to the Stage A call.

## Layer → guide (reference)

| Layer key | Guide |
| --- | --- |
| `unit` | `presets/agents/qa/guidelines/unit.md` |
| `functional` | `presets/agents/qa/guidelines/functional.md` |
| `integration` | `presets/agents/qa/guidelines/integration.md` |
| `component` | `presets/agents/qa/guidelines/component.md` |
| `contract` | `presets/agents/qa/guidelines/contract.md` |
| `e2e` | `presets/agents/qa/guidelines/e2e.md` |
| `visual-regression` | `presets/agents/qa/guidelines/visual-regression.md` |
| `mutation` | `presets/agents/qa/guidelines/mutation.md` |
| `ai-skill` | `presets/agents/qa/guidelines/ai-skill.md` |

The qa system prompt (`ai-mode prompt qa`) expects jev's layer when available; the
fallback router in `qa.md` is for missing Stage B only.
