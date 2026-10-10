## ux review
### Blocking
- No actual CLI surface is defined, so the spec cannot be reviewed as a user experience.
- The command shape is only partially specified: `ai-mode amigos --rounds N` is not enough to validate affordances, argument order, help, or error states.
- The layer plan and verdict are described but not shown in output shape, so the review cannot confirm whether the output is usable or complete.
- The uncertainty/parked/escalated state is described but not mapped to visible or actionable states in the CLI.
- The trigger model is undecided and unmodeled, so there is no UX for non-human callers yet.
### Non-blocking
- The roles and workflow are reasonably clear internally.
- The audit trail levels (`off`, `summary`, `full`) are logically defined.
- The layer table is internally consistent enough to be useful once rendered.
- The self-heal verdicts (`missing rules`, `missing examples`, `infeasible`) are conceptually sound.
### Proposed edits
- Define the CLI command structure before review:
  - Add positional and flag specs.
  - Show example invocations.
  - Define help text and error messages.
- Define the output format before review:
  - Specify whether the Example Map and layer plan are printed together, split, or interactive.
  - Define verdict and parked-question states explicitly.
- Define the uncertainty flow before review:
  - Add state names like `parked`, `re-asked`, `escalated`.
  - Add thresholds or rules for when each state changes.
- Define the trigger model before review:
  - Add authorized caller behavior, vetting rules, and input contract.
- If this is meant to be a CLI tool, add a minimal spec for the output surface so the review can validate usability.
