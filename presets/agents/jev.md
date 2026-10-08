---
role: jev
profile: any
model: jev
api: /v1/systemone
---

# jev

**Not a chat model.** This role is `ggml-org/Kev-4B-GGUF`, a System One **decision model**.

## Mission

Given the current user task (`state`), return typed choices with probabilities — no prose.

Orchestrators use **two separate calls** (never one combined question):

| Stage | When | Question key | Closed set |
| --- | --- | --- | --- |
| **A — role** | Every routed task | `role` | Specialist ini sections |
| **B — test layer** | Stage A chose `qa`, or the user explicitly asks for test strategy | `test_layer` | QA guideline layers (below) |

**Never** put role options and test-layer options in the same `questions.*.choice` — Kev expects one decision domain per request.

## How to call

Use the CLI — it builds the closed set, calls jev, applies the confidence policy below, and
records the call in `ai-mode events` / `ai-mode trace`:

```bash
ai-mode classify role  "<task>" --caller chief   # Stage A
ai-mode classify layer "<task>" --caller chief   # Stage B (after role=qa)
```

The closed set for Stage A is every ini section whose `agents/<role>.md` has a `summary:` line
(so `jev`, `fast`, and deprecated roles are excluded automatically).
**`chief` and `coder-xl` are deliberately not routable:** chief is the caller, and in testing it absorbed probability from real specialists (one research task was routed to `chief`); `coder-xl` overlaps `coder` and is an escalation chief chooses, not something a task description identifies. Stage B offers each layer
that has a guide in `presets/agents/qa/guidelines/`.

### Raw API (reference)

`POST {base_url}/systemone`, where `base_url` is `ai-mode url` and already ends in `/v1`
(do **not** append another `/v1`; that returns 404).

### Stage A — role

```json
{
  "model": "jev",
  "state": "User wants edge-case tests for login rate limiting.",
  "questions": {
    "role": {
      "type": "choice",
      "instructions": "Which specialist should handle this task?",
      "criteria": {
        "architect": "Plans, ADRs, large codebase structure",
        "coder": "Implement or change code per plan",
        "product": "Specs, stories, prioritization",
        "research": "Investigate APIs, spikes, competitive look",
        "docs": "README, changelog, release prose",
        "qa": "Test plans, cases, edge cases, regression strategy",
        "security": "Threat model, secure review",
        "design": "UI and visual critique",
        "ux": "Flows, usability, microcopy"
      }
    }
  }
}
```

Use the profile’s specialist names for `criteria` (learning-center / av-club differ). Omit roles that are not in the active preset ini.

**dev-shop:** Do **not** include **`fast`** in Stage A — it is an internal cheap-chat tool (Qwen Auto-mode classifiers, JSON helpers), not a human-facing specialist. Orchestrators call `fast` via `/v1/chat/completions` directly.

### Stage B — test layer (qa only)

Run **after** Stage A selects `qa` (or when routing straight to test strategy). Pass the user task plus brief context (file paths, stack, “component test for Button”, etc.) in `state`.

Each option maps to `presets/agents/qa/guidelines/<name>.md`:

| Layer key | Guide file | One-line criterion (use in `criteria`) |
| --- | --- | --- |
| `unit` | `unit.md` | Pure functions or single units in isolation, fast, no I/O |
| `functional` | `functional.md` | Feature or use-case through its public boundary/API |
| `integration` | `integration.md` | Modules with real collaborators (db, http, queues) |
| `component` | `component.md` | UI components rendered and exercised like a user |
| `contract` | `contract.md` | Consumer/provider API compatibility (e.g. Pact) |
| `e2e` | `e2e.md` | Full user journeys across the deployed system |
| `visual-regression` | `visual-regression.md` | Rendered UI pixel or snapshot diffs |
| `mutation` | `mutation.md` | Mutation testing: would the tests catch real bugs? Assertion strength, surviving mutants |
| `ai-skill` | `ai-skill.md` | Evals for AI skills, prompts, agent behavior |

```json
{
  "model": "jev",
  "state": "Add tests for checkout coupon validation in the Next.js API route handlers.",
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
        "mutation": "Mutation testing: would the tests catch real bugs? Assertion strength, surviving mutants",
        "ai-skill": "Evals for AI skills, prompts, agent behavior"
      }
    }
  }
}
```

Hand the winning layer (and optional second if policy below) to **qa** — qa loads guides and authors cases; qa does **not** re-run layer classification when jev already chose.

### Layer selection policy (orchestrator)

- **High confidence** (clear winner vs. rest): load **one** matching guide for qa.
- **Low confidence** or **top two within ~0.1 probability**: load **top two** guides, or have **chief** ask **one** clarifying question and re-run Stage B.
- Mixed tasks: primary layer from jev first; qa may add **at most one** adjacent layer from the fallback table in `qa.md` if the task clearly spans layers — do not replace jev’s primary pick.

## Tuning jev's confidence

Confidence depends mostly on (1) how many overlapping options are in the closed set and (2) whether
each `summary:` uses the words people actually use in task descriptions. Measure, don't guess:

```bash
make eval        # ai-mode classify --eval on evals/classify.jsonl and the held-out set
```

Tune `summary:` lines (and the layer criteria in `internal/app/classify.go`) against
`evals/classify.jsonl`; run `evals/classify-holdout.jsonl` afterwards to check the gains generalise.
Watch **wrong-but-clear** (confidently wrong: the dangerous one) before average confidence.

## Rules for orchestrators

- Always pass a **closed set**; never ask Jev to invent a new role or layer name.
- Prefer the highest-probability option above a confidence threshold; otherwise ask the chief to clarify with the human.
- Do not use `/v1/chat/completions` for this model.

## Example Stage B curl (what `ai-mode classify layer` sends)

```bash
BASE="$(ai-mode url)"
curl -sS "${BASE}/systemone" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "jev",
    "state": "User asked for Playwright coverage of the signup funnel.",
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
          "mutation": "Mutation testing: would the tests catch real bugs? Assertion strength, surviving mutants",
          "ai-skill": "Evals for AI skills, prompts, agent behavior"
        }
      }
    }
  }'
```
