---
name: ai-mode-routing
description: "Consult an ai-mode specialist (product, security, research, ux, design, qa, docs, architect, coder) from chief. Use when the human says ask the product agent, route to specialist, talk to security, ai-mode role, or names any dev-shop specialist for an ad-hoc question — not for spec-specialist-review (use that skill) or brainstorming."
---

# ai-mode routing

Call **real llama-server roles** on the active ai-mode profile from a **chief** session. The human stays on chief; you orchestrate **`ai-mode ask`** (preferred) or a brief `/model` switch.

<HARD-GATE>
**Same-turn Shell MUST run `ai-mode ask` before this turn ends.**

These do **NOT** count as calling the specialist:
- Invoking the Skill tool / “already loaded in context” / reading this file
- **`AskUserQuestion`** loops instead of the model
- Announcing “Using ai-mode-routing…” without Shell

You are **forbidden** to:
- End the turn after loading this skill without having run **Shell**:
  `ai-mode ask <role> "<question>"`
  (quote the question; add `--max-tokens` if needed).
- Write memory / “specialist answered” / synthesize for the human until **`ai-mode ask` exits 0** with **non-empty stdout**.
- Substitute chief imagination, **`Task`**, **`Agent`**, or **`Skill(using-superpowers)`** for the specialist call.
- **Invent** specialist answers when the model did not run.

If **`ai-mode ask` fails** (non-zero exit, empty stdout): report the failure to the human and retry or use **`/model <role>`** fallback — **do not** pretend the specialist ran.

**Even if this skill is already loaded**, run **`ai-mode ask`** immediately in the same turn — do not re-invoke the Skill tool or ask the human product questions chief could send to **`product`** via **`ai-mode ask`**.

This skill is for **single ad-hoc consultations** and open-ended “who should handle this?” routing.

- **NOT** for the fixed **`spec-specialist-review`** six-pass pipeline → use `/superpowers:spec-specialist-review`.
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
- Shell: `eval "$(ai-mode env)"` exports `OPENAI_BASE_URL` (optional for `ask`; it uses the active profile)
- Jev endpoint (role unclear only): **`$(ai-mode url)/systemone`**

## Mandatory workflow (Qwen Code)

**Order is fixed. Do not skip steps.**

### Step 1 — Announce (only)

Say you are using ai-mode-routing and which `<role>` you will call. **Do not synthesize yet.**

### Step 2 — Shell: `ai-mode ask` (REQUIRED)

```bash
ai-mode ask product "Human question plus minimal context — no monorepo globs on chief" --max-tokens 2048
```

Piped stdin when the question is long:

```bash
echo "one sentence on MVP scope" | ai-mode ask product --max-tokens 256
```

**Gate:** Proceed only if exit code is **0** and stdout is **non-empty**. If not, stop and fix (server down, wrong role) — no specialist attribution.

**Emergency fallback** (only if `ai-mode ask` is unavailable): `SYSTEM=$(ai-mode prompt "$ROLE")` and curl `$(ai-mode url)/chat/completions` with jq — same JSON shape as before. Prefer **`ai-mode ask`** always.

### Step 3 — Pick the role (when unclear)

| Human intent | Routing |
| --- | --- |
| Names a role (`product`, `security`, …) | Use that role; **skip jev** → go to Step 2 |
| “Which specialist?” / domain unclear | **jev Stage A** first (below), then Step 2 with chosen role |
| jev returns **`qa`** | **jev Stage B** for `test_layer`, load `presets/agents/qa/guidelines/<layer>.md`, then Step 2 with **`qa`** |

### jev Stage A (only when role unclear)

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

Use the highest-probability role, then **Step 2** with that `ROLE`.

### Step 4 — Fallback — `/model`

Only if Shell/`ai-mode ask` is blocked or repeatedly fails after you reported the error: `/model <role>`, ask with that role’s behavior, then **`/model chief`** before the next turn.

### Step 5 — Respond as chief

Summarize **`ai-mode ask` stdout** for the human. Attribute clearly (“**product** suggests …”). Do not claim a specialist ran if Step 2 did not return valid content.

## Qwen Code checklist (mandatory)

1. Invoke this skill; announce role — **stop** (no synthesis, no memory update about the answer).
2. **Run Shell** — `ai-mode ask <role> "<question>"` (see Step 2).
3. Verify **exit 0** + non-empty stdout.
4. **Then** synthesize on **chief** — same turn, after step 3.

**Forbidden before step 3 completes:** ending the turn, updating memory that the specialist answered, **`AskUserQuestion`** instead of the model (unless the human’s question is literally empty), or attributing quotes to product/security/etc.

## Related

- Chief system prompt: `presets/agents/chief.md` — **Ad-hoc specialist routing**
- Fixed spec pipeline: `skills/spec-specialist-review/SKILL.md`
- Jev contract: `presets/agents/jev.md`
- QA layers after jev: `skills/testing-guidelines/SKILL.md`

## Principles

- **One real model per consultation** — weights and system prompt come from ai-mode.
- **Chief synthesizes** — specialists do not talk to the human directly unless the human `/model` switched.
- **Context discipline** — large reads → **`architect`** via **`ai-mode ask architect "…"`**, not chief globs.
