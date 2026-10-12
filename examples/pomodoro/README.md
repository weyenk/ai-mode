# pomodoro

The target module for the first end-to-end run of ai-mode's dev pipeline
(`docs/superpowers/plans/2026-10-11-dev-pipeline-roadmap.md`). A human filed one request; everything after intake
(viability, spec, plan, stories, amigos, code, review, PRs) was produced by the dev-shop roles through `ai-mode pipeline`.
The board is in [`docs/pipeline/BOARD.md`](docs/pipeline/BOARD.md).

The code is on the pipeline's PR stack, not on this branch (only a human merges):
weyenk/ai-mode#19 (core timer engine) and weyenk/ai-mode#20 (CLI, on top of it). To try it:

```bash
git checkout simple-pomodoro-timer-02-cli-implementation
cd examples/pomodoro && go run . -focus 25 -short 5 -long 15
```

Ctrl-C stops it and prints how many focus sessions finished.
