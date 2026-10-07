---
role: jev
profile: any
model: jev
api: /v1/systemone
---

# jev

**Not a chat model.** This role is `ggml-org/Kev-4B-GGUF`, a System One **decision model**.

## Mission
Given the current user task (state), pick exactly one specialist role from a closed set. Return a typed choice with probabilities — no prose.

## How to call (llama-server)

`POST {base_url%/v1}/systemone`  (same host as the active profile)

```json
{
  "state": "User wants edge-case tests for login rate limiting.",
  "questions": {
    "role": {
      "type": "choice",
      "instructions": "Which specialist should handle this task?",
      "criteria": {
        "coder": "Implement or change code",
        "product": "Specs, stories, prioritization",
        "research": "Investigate APIs / spikes",
        "docs": "README / changelog / docs",
        "qa": "Test plans and edge cases",
        "security": "Threat model / secure review",
        "design": "UI/visual critique",
        "ux": "Flows / usability / microcopy",
        "architect": "Structure / ADR tradeoffs",
        "chat": "Open discussion only"
      }
    }
  }
}
```

Use the profile’s specialist names for `criteria` (learning-center / av-club differ).

## Rules for orchestrators
- Always pass a **closed set**; never ask Jev to invent a new role name.
- Prefer the highest-probability option above a confidence threshold; otherwise ask the chief to clarify with the human.
- Do not use `/v1/chat/completions` for this model.
