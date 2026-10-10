## ux review
### Blocking
- The CLI surface does not specify whether `--json` and `--audit full` produce overlapping or incompatible outputs.
- The `--resume <ID>` behavior is not clear enough: which files are loaded, in what order, and whether the command aborts or continues mid-loop.
- Uncertainty thresholds and sampling counts are not defined, so the parked/re-asked/escalated logic is under-specified.
- Exit code `3` for `paused` is not explained in the help text or CLI surface.
- The Story Contract and layer plan are not clearly distinguished in the output order; a human reading stdout may not scan the artifact correctly.

### Non-blocking
- The spec is internally consistent enough for a small team to implement.
- The CLI flags are mostly usable, though not refined enough for a polished product surface.
- The audit trail structure is sound, but the help text is not detailed enough to be self-documenting.

### Proposed edits
- Add a `--json` output section to the help text that describes whether JSON artifacts replace or augment plain-text output.
- Add a `--resume` section to the help text that explains what state is loaded and whether the loop continues from the current role or resets.
- Define uncertainty defaults in the spec, e.g.:
  - Sample count: `3`
  - Park threshold: `0.7`
  - Score blend: `self-consistency 70%, logprob 30%, disagreement tie-break`
- Add a line to the exit codes section explaining `3 paused`.
- Change the default output order to:
  1. verdict
  2. layer plan
  3. questions
  4. Story Contract
- Clarify that `contract.json` is the authoritative artifact and `decisions.jsonl` is the audit trail.
