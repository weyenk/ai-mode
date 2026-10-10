## architect review (grounded)

### Blocking
- **Missing Command Registration**: The `ai-mode amigos` command is not registered in `internal/app/cli.go` within the `commands()` map. This prevents the CLI from recognizing the new entry point.
- **Missing Skill Implementation**: The `skills/three-amigos/SKILL.md` mentioned in the spec does not exist in the provided `skills/` directory listing. Without this, the "trigger is a skill invocation" requirement cannot be met.
- **Missing State Management Logic**: While `internal/app/team.go` shows an existing pattern for writing to `stateDir()`, there is no visible implementation in `internal/app/` for managing the `amigos/<meeting_id>/` directory structure, `contract.json`, or the specific JSON schemas required for the Story Contract.

### Non-blocking
- **Missing Subcommands**: The `ai-mode plan lint` and `ai-mode plan tdd-check` commands mentioned in the "Repo changes implied" section are not present in `internal/app/cli.go`.
- **Missing Infrastructure**: The `gh-stack` extension and the `ai-mode stack` command are not yet implemented in the codebase.
- **Missing Configuration**: The `presets/amigos-callers.toml` file (for the caller allowlist) is not present in the context.
- **Unverified Agent Prompts**: The spec requires updates to `presets/agents/product.md` and `presets/agents/qa.md`, but only `presets/agents/architect.md` was provided for review.

### Proposed edits
- **Update `internal/app/cli.go`**: Add `"amigos": {"description", cmdAmigos}` to the `commands()` map.
- **Implement `cmdAmigos`**: Create a new command function (likely in `internal/app/cmds.go` or a new `internal/app/amigos.go`) that implements the loop controller, parses the required flags (`--story`, `--rounds`, `--audit`, `--resume`, etc.), and handles the `amigos/<meeting_id>/` state lifecycle.
- **Implement the Skill**: Create `skills/three-amigos/SKILL.md` to wrap the `ai-mode amigos` command as specified.
- **Schema Definition**: Define the JSON schemas for `contract.json`, `decisions.jsonl`, and `parked.jsonl` (unverified) to ensure the "structured intake" and "machine-friendly" requirements are enforceable.
- **Expand Agent Presets**: Create/update `presets/agents/product.md` and `presets/agents/qa.md` with the "Amigos meeting" section to ensure the roles can react to the new turn format.
