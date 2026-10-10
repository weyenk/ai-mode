## architect review

### Blocking

* **CLI Plumbing & Routing Integration**: The spec explicitly states it is "blocked on CLI plumbing." Since the `ai-mode amigos` command and its integration with `ai-mode-routing` are not yet implemented, a concrete implementation plan for the logic cannot be written. We cannot plan the "skill" implementation without the underlying command-line infrastructure and the routing capability to orchestrate the Product $\rightarrow$ QA $\rightarrow$ Coder sequence.
* **State Persistence Mechanism**: The design requires maintaining "current Example Map + delta since that role last spoke" across rounds. The spec does not define how this state is persisted between discrete CLI invocations. Without a defined session/state management strategy (e.g., a local `.ai-mode/sessions/` directory or a specific JSON/SQLite backend), the implementation of the state machine is unachievable.
* **Output Schema (The "Contract")**: The `coder` and `qa` roles rely on generating an "Example Map" and "Layer Plan." The structural schema (JSON or Markdown format) for these artifacts is not defined. Without a schema, we cannot design the prompts for the agents or the parsing logic required for the `ai-mode amigos` summary.

### Non-blocking

* **Uncertainty Signal Source**: The reliance on `llama-server` logprobs is speculative. However, the spec provides a fallback (self-consistency/cross-role disagreement), so this does not block the logic implementation.
* **Agent Authorization Model**: While the trigger/authorization model for non-human callers is "undecided," the design can proceed with a human-centric model and extend to agent-based inputs later.
* **Audit Level Sink**: The destination for the "extreme observability" hash chain and sink directory can be finalized during the implementation phase.

### Proposed edits

* **Define Session Lifecycle**: Add a section defining how a "session" is initiated, how a `session_id` is tracked in the CLI, and where the "working context" resides on disk.
* **Define Artifact Schemas**: Provide a draft JSON schema for the `Example Map` (containing the decision log and pending scenarios) and the `Layer Plan` (mapping scenarios to the 1-10 PR layers). This is critical for the `coder` to know what to produce.
* **Formalize the Input Contract**: Explicitly define the "Intake contract" structure (story text, provenance, etc.) to allow the design of the routing/validation logic.
* **Specify Decision Log Structure**: Detail what a "decision log" entry looks like (e.g., `timestamp`, `role`, `decision`, `rationale`, `impacted_layers`) to ensure the `summary` audit level is implementable.
