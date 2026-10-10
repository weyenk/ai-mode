## research review
### Blocking
- Logprobs availability in llama-server is unresolved and blocks the uncertainty design. The spec states it is a blocker requiring a spike ("Logprobs availability in llama-server (research: blocking for the uncertainty design; do a spike before planning, fallback is self-consistency + disagreement)"), but provides no evidence that logprobs are exposed or usable. The claim "token logprobs at answer points if llama-server exposes them" depends on an unverified external dependency with no fallback evidence.

### Non-blocking
- Vikunja trial findings are stated with evidence and limits: SQLite failed under concurrency (20 parallel writes → `database is locked`), Postgres succeeded (zero errors in same load). The decision to use Postgres is supported by trial results and documented constraints (SQLite requires a single-writer gate).
- GitHub stacked PR mechanics are referenced without unsupported claims; the layer table aligns with GitHub docs (links provided), and "layers 8/9 skipped until harnesses exist" is explicitly stated as an implementation note.
- AI-eval placement (layer 8 above e2e) is a stated design rationale ("AI evals test nondeterministic dependency... sit above e2e and below performance"), not an unsupported claim.

### Proposed edits
- Remove the duplicate open item "Whether llama-server exposes logprobs usable for the uncertainty signal" from the "Open" section. The existing item "Logprobs availability in llama-server..." already covers this with context, spike requirement, and fallback plan.  
- Add a footnote to the uncertainty scoring description: "Fallback: self-consistency + cross-role disagreement if logprobs unavailable (per spec's 'fallback' clause)."

Sources / references  
- Spec section: Uncertainty, Open, Vikunja trial findings, AI-eval placement rationale
