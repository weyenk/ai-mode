---
name: writing-specs
description: "Stage 4 of the dev pipeline, in place of Superpowers brainstorming: architect decides how many specs a request needs and writes each one as an agent (fixed headings, user flows, numbered Given/When/Then scenarios), the six specialists review it (spec-specialist-review order), and chief merges the reviews. Runs with `ai-mode pipeline stage <card>`; no human approval step (roadmap D2)."
---

# Writing specs (stage 4)

1. A request at stage `spec`: `ai-mode pipeline stage <request card> --push` has architect split it into spec cards.
2. Each spec card at stage `spec`: `ai-mode pipeline stage <spec card> --push`
   - architect writes `docs/pipeline/specs/<card>-<slug>.md` with the headings Goal, Users and success metrics, Scope,
     User flows, Behaviour rules, Scenarios (`- **S1 — name.** Given …, when …, then …`), Global Constraints, Technical
     approach, Open questions; a check enforces them;
   - product → research → security → ux → design → architect review it (Blocking / Non-blocking / Proposed edits, saved under `specs/reviews/`);
   - chief merges as an agent that may write only the spec, keeps scenario ids stable and adds a Review log.
3. The card moves to `planning`. Report the scenario count and the blocking items chief applied or declined.

Scenario ids are stable from here on: the feature plan assigns each to exactly one story.
