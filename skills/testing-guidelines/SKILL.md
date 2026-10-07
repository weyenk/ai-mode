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

### Invoking `/v1/systemone`

Qwen Code may not have a native System One tool. Options:

- **Chief orchestrates**: chief runs Stage A then Stage B and passes `test_layer` into the
  qa turn.
- **curl** (active router from `ai-mode url`):

```bash
BASE="$(ai-mode url)"
curl -sS "${BASE}/v1/systemone" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "jev",
    "state": "<user task + brief context>",
    "questions": {
      "test_layer": {
        "type": "choice",
        "instructions": "Which test layer should qa use as the primary lens?",
        "criteria": {
          "unit": "Pure functions or single units in isolation, fast, no I/O",
          "functional": "Feature or use-case through its public boundary/API",
          "integration": "Modules with real collaborators (db, http, queues)",
          "component": "UI components rendered and exercised like a user",
          "contract": "Consumer/provider API compatibility (e.g. Pact)",
          "e2e": "Full user journeys across the deployed system",
          "visual-regression": "Rendered UI pixel or snapshot diffs",
          "mutation": "Assertion strength, surviving mutants",
          "ai-skill": "Evals for AI skills, prompts, agent behavior"
        }
      }
    }
  }'
```

There is no `ai-mode classify` helper yet — use curl or chief-mediated jev calls.

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
