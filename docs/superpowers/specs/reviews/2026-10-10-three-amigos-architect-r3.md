## architect review

### Blocking
*   **Role System Prompts/Instructions:** While the spec defines the roles, their responsibilities, and the use of delimiters (e.g., `<story_data>`), the actual content/instructions for the `product`, `qa`, and `architect` prompts are missing. A developer cannot implement the `internal/app` loop controller or the skill wrapper without the specific system instructions that define how these roles react to each other or how they should format their internal reasoning.
*   **Uncertainty Calculation Logic:** The spec defines the components (agreement ratio, logprobs, cross-role disagreement) but lacks the mathematical formula for "blended" uncertainty. A coder cannot implement the threshold logic (e.g., `if uncertainty > threshold then park`) without a concrete algorithm or weight distribution for these variables.
*   **Intake Gate Error Specification:** The spec mentions the gate "rejects malformed contracts" and defines exit code 1 for unauthorized callers, but it does not specify the error response format or the specific validation rules (beyond "strict schema validation") required for the `intake_gate` to prevent prompt injection or malformed data.

### Non-blocking
*   **JSON Schemas:** The spec identifies exact schemas as a "planning deliverable," so the absence of formal JSON Schema definitions is acceptable at this stage, provided they are included in the upcoming `writing-plans` phase.
*   **`gh-stack` Installation:** The dependency on `gh-stack` and the need for human approval for installation is a clear operational step that does not block the design of the `amigos` command itself.
*   **`jev Stage B` Integration:** The reliance on the existing `jev Stage B` for test level selection is acceptable, assuming the interface for retrieving those levels is stable.
*   **Audit Sink/Redaction:** The technical details of the "redaction pass" (regex patterns, etc.) are deferred to the implementation plan.

### Proposed edits
*   **Define "Blended" Uncertainty:** Explicitly define the formula for the uncertainty score (e.g., `Score = (w1 * AgreementRatio) + (w2 * LogprobConfidence)`) to ensure the loop controller can execute the "park/re-ask/escalate" logic deterministically.
*   **Prompt Template Reference:** Add a requirement to the `writing-plans` phase to define the prompt templates for each role, or include them as part of the `internal/app` implementation task.
*   **Clarify Error Handling:** Define the structure of the error output for the `intake_gate` (e.g., a structured `error_report` JSON) so that calling skills can programmatically react to specific validation failures (e.g., "unauthorized_caller" vs "schema_mismatch").
