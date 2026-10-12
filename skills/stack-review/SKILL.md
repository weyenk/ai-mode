---
name: stack-review
description: "Stage 9 of the dev pipeline: architect reviews each of a story's stack layers, bottom first, from the layer's diff and the Story Contract; blocking comments are fixed in the owning layer (coder for code, qa for tests), layers above are rebased, and the loop ends approved or, after three rounds, with a human. Runs with `ai-mode pipeline stage <story card>`."
---

# Stack review (stage 9)

1. The card is a story at `architect-review` (stage 8 left a linear stack of layer branches and a passing `plan tdd-check`).
2. Run `ai-mode pipeline stage <story card> --push`.
3. Per layer, architect replies with `{"approve": …, "comments": [{"file", "line", "severity": "blocking|nit", "comment"}]}`,
   saved as `docs/pipeline/reviews/<card>/layer-<n>-round-<r>.json`. Blocking comments are fixed on that layer's branch
   (commit trailer `Phase: review`) and every layer above is rebased onto the fix.
4. Exit: no blocking comments on any layer, `go test ./...` green at the tip, `ai-mode plan tdd-check` passes → `handoff`.
   Three rounds without approval → `needs-human` with the open comments.

Never fix a lower layer's problem from a higher branch, and never edit a test to make it pass unless architect's comment is about the test.
