## research review
### Blocking
None. The revised stack design aligns with GitHub's guidance (splitting by feature dependency, not test types) as explicitly stated in the spec and verified against the cited tutorial. CI-gate claims are consistent with the spec's declaration of `not-available` states for AI evals/performance without harnesses, and version requirements are met (gh 2.102.0 ≥ 2.90+ and 2.36+, git 2.56.0 ≥ 2.20+).

### Non-blocking
- **Logprobs availability in llama-server**: Spec states this is an open item requiring a spike ("logprobs availability in llama-server (research: blocking for the uncertainty design; do a spike before planning; fallback is self-consistency + cross-role disagreement, already specified)"). This is documented as a known gap with a defined fallback, not a misstated claim.

### Proposed edits
None. All claims about stack design (GitHub alignment), CI gates (states and artifact ownership), and tooling versions are accurately reflected in the spec using cited sources (tutorial, gh-stack README) and machine state (gh 2.102.0, git 2.56.0). No unsupported assertions exist.

Sources / references:  
- GitHub tutorial: `docs.github.com/en/copilot/tutorials/stack-ai-generated-code-in-pull-requests` (cited in spec)  
- gh-stack README: "gh and Git 2.36+" (spec cites this directly)  
- Machine state: "this machine has gh 2.102.0, git 2.56.0" (spec states factually)
