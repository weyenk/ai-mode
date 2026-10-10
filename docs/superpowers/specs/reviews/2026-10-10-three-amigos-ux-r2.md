## ux review
### Blocking
- The CLI surface does not clearly communicate the command’s primary goal in one scan. The help text reads like a dense spec rather than a usable operator guide.
- Exit codes are not explained in the surface, so `0`, `2`, and `3` are opaque to operators unfamiliar with the tool.
- The `--audit` flag has inconsistent behavior depending on whether `--json` is used, and the distinction between `summary` and `full` is not obvious.
- The `--resume` argument is not clearly tied to state persistence, so operators may not know whether a previous run can be continued.
- The `layer_plan[]` authorship model is not visible in the CLI output, so the handoff logic is hidden from the operator.

### Non-blocking
- The spec is internally consistent enough to feel credible.
- The CLI has enough positional and flag flexibility for advanced users.
- The output structure is logically sound, even if not yet polished into a human-readable surface.

### Proposed edits
- Add a one-line summary to the help text that explains the command’s purpose, e.g.:
  `ai-mode amigos` runs a structured review of one story with product, qa, and architect roles before generating a test plan.
- Replace the current help block with clearer sections:
  - `Purpose`
  - `Inputs`
  - `Output`
  - `Exit codes`
  - `Flags`
  - `State persistence`
  - `Examples`
- Define exit codes explicitly in the help text:
  - `0`: ready
  - `2`: not-ready
  - `3`: paused
  - `1`: error
- Make `summary` and `full` more visible:
  - `--audit summary`: minimal artifacts, no transcript
  - `--audit full`: full prompt/response log plus transcript
- Clarify `--resume`:
  - `--resume <id>` continues a paused meeting from `map.json`, `decisions.jsonl`, and `parked.jsonl`
- Clarify `layer_plan[]` ownership:
  - `qa` proposes the plan; `architect` revises it for feasibility; `writing-plans` consumes it
- Add a simple example section to the help text:
  ```bash
  ai-mode amigos --story "Update user profile with avatar upload"
  ai-mode amigos --story my-story.txt --rounds 2 --audit summary
  ai-mode amigos --resume 7f8a2c3d-4e5f-6789-0123-456789abcdef
  ```
- Tighten intake precedence into a clearer rule:
  1. positional string
  2. `--story -`
  3. `--story FILE`
  4. JSON contract
- If the tool is meant for operators, add a plain-text default output format that shows verdict first, then layer plan, then questions, then map. Right now the output order is close, but not clear enough.
