---
name: ai-mode-routing
description: "Consult an ai-mode specialist (product, security, research, ux, design, qa, docs, architect, coder) from chief. Use when the human says ask the product agent, route to specialist, talk to security, ai-mode role, or names any dev-shop specialist for an ad-hoc question — not for spec-specialist-review (use that skill) or brainstorming."
---

# ai-mode routing

Call **real llama-server roles** on the active ai-mode profile from a **chief** session. The human stays on chief; you orchestrate HTTP (preferred) or a brief `/model` switch.

<HARD-GATE>
This skill is for **single ad-hoc consultations** and open-ended “who should handle this?” routing.

- **NOT** for the fixed **`spec-specialist-review`** six-pass pipeline → use `/superpowers:spec-specialist-review`.
- **NOT** satisfied by invoking **`using-superpowers`** again, **`Task`**, or **`Agent(general-purpose)`** in the background.
- **NEVER** `Task(subagent_type: "product"|"security"|…)` — those strings are **model ids**, not Qwen subagent types.
</HARD-GATE>

## When to use

Trigger when the human (or chief context) says things like:

- “Ask the **product** agent about …”
- “What would **security** think of …?”
- “Route this to a specialist” / “consult **ux**”
- “Use the **research** model on …”
- Any explicit **ai-mode role name** from the active preset for a **one-off** question (not the spec review gate)

If the human already named **`spec-specialist-review`** or an approved design spec path for multi-pass review, use **`spec-specialist-review`** instead.

## Prerequisites

- Active profile: `ai-mode which` / `ai-mode doctor`
- Shell: `eval "$(ai-mode env)"` exports `OPENAI_BASE_URL`
- **`ai-mode url`** prints the OpenAI base including **`/v1`** (e.g. `http://127.0.0.1:8080/v1`)
- Chat endpoint: **`$(ai-mode url)/chat/completions`**
- Jev endpoint: **`$(ai-mode url)/systemone`**

## Workflow

### 1. Pick the role

| Human intent | Routing |
| --- | --- |
| Names a role (`product`, `security`, …) | Use that role; **skip jev** |
| “Which specialist?” / domain unclear | **jev Stage A** — POST `systemone` with `model: jev`, `questions.role` criteria from `presets/agents/jev.md` (profile’s ini sections only) |
| jev returns **`qa`** | **jev Stage B** for `test_layer`, load `presets/agents/qa/guidelines/<layer>.md`, then call **`qa`** |

### 2. Call the specialist (preferred — chief stays active)

```bash
eval "$(ai-mode env)"
ROLE=product   # example
SYSTEM=$(ai-mode prompt "$ROLE")
QUESTION='…'   # human ask + minimal context; no monorepo globs on chief

curl -sS "$(ai-mode url)/chat/completions" \
  -H "Content-Type: application/json" \
  -d "$(jq -n \
    --arg model "$ROLE" \
    --arg system "$SYSTEM" \
    --arg user "$QUESTION" \
    '{model: $model, messages: [{role:"system", content:$system}, {role:"user", content:$user}]}')"
```

Parse the assistant `content` from the JSON response.

### 3. jev Stage A (only when role unclear)

Minimal example — adjust `state` and `criteria` to the active profile:

```bash
eval "$(ai-mode env)"
curl -sS "$(ai-mode url)/systemone" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "jev",
    "state": "<clean task statement>",
    "questions": {
      "role": {
        "type": "choice",
        "instructions": "Which specialist should handle this task?",
        "criteria": {
          "product": "Specs, stories, prioritization",
          "research": "APIs, spikes, competitive look",
          "security": "Threat model, secure review",
          "ux": "Flows, usability",
          "design": "UI and visual critique",
          "qa": "Tests and edge cases",
          "docs": "Documentation prose",
          "architect": "Plans and large codebase mapping",
          "coder": "Implementation per plan"
        }
      }
    }
  }'
```

Use the highest-probability role (or orchestrator policy from jev output), then step 2 with that `ROLE`.

### 4. Fallback — `/model`

If curl/HTTP is unavailable: `/model <role>`, ask with that role’s behavior, then **`/model chief`** before the next turn.

### 5. Respond as chief

Summarize the specialist answer for the human. Attribute clearly (“**product** suggests …”). Do not claim a specialist ran if only chief or a Qwen subagent replied.

## Qwen Code checklist

1. Invoke this skill (announce: “Using ai-mode-routing to consult `<role>`”).
2. Run shell **curl** to **`$(ai-mode url)/chat/completions`** with **`ai-mode prompt <role>`** — do **not** open a background **Task** or **Agent**.
3. Synthesize on **chief**.

## Related

- Chief system prompt: `presets/agents/chief.md` — **Ad-hoc specialist routing**
- Fixed spec pipeline: `skills/spec-specialist-review/SKILL.md`
- Jev contract: `presets/agents/jev.md`
- QA layers after jev: `skills/testing-guidelines/SKILL.md`

## Principles

- **One real model per consultation** — weights and system prompt come from ai-mode.
- **Chief synthesizes** — specialists do not talk to the human directly unless the human `/model` switched.
- **Context discipline** — large reads → **`architect`** via the same HTTP pattern, not chief globs.
