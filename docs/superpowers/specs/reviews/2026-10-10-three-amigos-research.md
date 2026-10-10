## research review
### Blocking
- Whether llama-server exposes usable token logprobs for uncertainty signal (explicitly listed in spec's "Open" section). Spec states: *"token logprobs at answer points if llama-server exposes them (to verify)"* but provides no confirmation of availability. This blocks implementation of the uncertainty mechanism described in the Uncertainty section.
### Non-blocking
- GitHub stacked PR mechanics research pending (per spec's note: *"See GitHub stacked PR docs... read before specifying stack mechanics"*). Not a claim, but a pending task for delivery design.
### Proposed edits
- None (spec already correctly flags logprobs exposure as an open point in "Open" section; no unsupported claims exist).
