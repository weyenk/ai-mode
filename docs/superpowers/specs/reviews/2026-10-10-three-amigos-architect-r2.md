## architect review

### Blocking
* **Skill Allowlist Authority:** The spec lists the location and management of the "skill allowlist" (required for the security gate) as **Open**. The `internal/app` implementation cannot complete the authorization logic (checking `provenance.caller_id` against the allowlist) without knowing if this is a static file in the repo, a dynamic config in `internal/app`, or a database entry in Vikunja. This prevents the definition of the security-layer interfaces.

### Non-blocking
* **Uncertainty Signal (Logprobs):** While the availability of `llama-server` logprobs is "Open," the design includes a functional fallback (self-consistency and cross-role disagreement), allowing the scoring logic to be implemented as a modular component.
* **JSON Schema Definitions:** The exact schemas for the intake contract and Example Map are marked as "planning deliverables." This is acceptable for `writing-plans` as long as the required fields are identified (which they are).
* **Round Hard Cap:** The specific value for the "hard maximum" to prevent runaway loops is deferred to the plan.
* **Companion-service:** The lack of specification for the generic per-profile service concept is explicitly deferred and does not impact the `amigos` core logic.

### Proposed edits
* **Loop Controller Ownership:** Clarify in the `internal/app` section that the "loop controller" (the logic driving re-runs, round counting, and the state machine transitions) resides within the `internal/app` command itself, whereas the skill wrapper is strictly a "thin" entry point for routing and payload parsing. This prevents the skill from becoming too heavy.
* **Uncertainty Configuration:** Explicitly define the "uncertainty threshold" as a configurable parameter (via flag or config) in the `internal/app` surface to avoid magic numbers in the implementation.
* **State Persistence:** Ensure the plan specifies whether the `amigos/<meeting_id>/` directory structure is managed by the `internal/app` command or a separate lifecycle utility.
