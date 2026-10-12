---
name: story-breakdown
description: "Stage 6 of the dev pipeline: turn a lint-clean feature plan into story files and story cards, in stack order, each worded by product as a user story and carrying its spec scenarios, the global constraints and the plan's contracts. Runs with `ai-mode pipeline stage <plan card>`; the files are what `ai-mode amigos --story` reads."
---

# Story breakdown (stage 6)

1. The card is a plan at stage `planning` whose feature plan passes `ai-mode plan lint --feature`.
2. Run `ai-mode pipeline stage <plan card> --push`.
3. It writes `docs/pipeline/stories/<card>-<slug>.md` per story (Story, Scope, Scenarios from the spec, Global
   Constraints, Contracts) and a story card at `amigos`, ordered so dependencies come first (that order is the stack order).
4. Report the story cards and which scenarios each carries. Each goes to the three amigos next.
