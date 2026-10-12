---
name: viability
description: "Stage 3 of the dev pipeline: decide whether a new request card is worth building. Runs research, product and architect (plus security for health or enterprise requests) through `ai-mode pipeline stage <card>`; go/no-go and ROI are computed from the role estimates by a context rubric, not by a model."
---

# Viability (stage 3)

1. The card must be a request at stage `new` (`ai-mode pipeline show <card>`).
2. Run `ai-mode pipeline stage <card> --push`.
3. Read the report it names (`docs/pipeline/viability/<card>-<slug>.md`) and the card's new note:
   - **GO** → the card is at `spec`; architect writes the spec next.
   - **NO-GO** or **no decision** → the card is `needs-human`. Show the failed checks; the human confirms the no-go (the card
     is marked `rejected`) or edits the request and runs `ai-mode pipeline retry <card>`.

Each role ends its reply with a fixed JSON estimate (research: feasible, confidence; product: value 1–5, success metrics,
MVP scope; architect: effort days, complexity; security: risk 1–5). ROI = value / effort days. The rubric per context
(hobby, enterprise, health, other) sets the thresholds; see `internal/app/pipeline_viability.go`. Do not recompute or
override the decision by hand.
