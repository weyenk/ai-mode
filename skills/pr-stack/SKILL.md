---
name: pr-stack
description: "Build, fix, restack and land a stack of dependent pull requests with the gh-stack CLI. Use when a Story Contract has a layer_plan, or the human asks to stack a feature into reviewable layers. Local branch work is automatic; pushing, opening PRs, merging and unstacking always need the human's explicit yes."
---

# PR stack (gh-stack wrapper)

Drive one stack, bottom layer first, with `gh stack` (GitHub's stacked-PR CLI extension). Layers follow
**feature dependency** (foundation at the bottom), not test types; each layer is one coherent,
quick-to-review change **plus the tests for that slice, written first**. See the Delivery section of
`docs/superpowers/specs/2026-10-10-three-amigos-design.md`.

Command source: https://github.com/github/gh-stack (README). Do not use flags that are not listed here;
check `gh stack <cmd> --help` if unsure.

<HARD-GATE>
**Never run these without the human's explicit yes in this turn, naming the action:**
`gh stack push`, `gh stack submit`, `gh stack sync` (it pushes), `gh stack link`, `gh stack merge`,
`gh stack unstack`, `gh extension install`. They publish branches or PRs, merge, or change GitHub state.
A yes to one does not cover the next. Opening the skill with an approved layer plan authorizes only
**local** work: creating branches and committing in this clone.

**Never run interactive commands** (they open prompts or full-screen UIs an agent cannot drive):
`gh stack init` with no branch, `gh stack add` with no branch, `gh stack switch`, `gh stack modify`,
`gh stack checkout` with no argument, `gh stack submit` without `--auto`, `gh stack merge` without `--yes`.
Without a terminal these fail fast instead of hanging (verified for `init`, `add`, `switch`, `modify`, `checkout`), but do not rely on that: a harness with a pseudo-terminal may prompt. If a command would prompt, stop and tell the human.
</HARD-GATE>

## Preconditions

1. `gh stack --help` works. If not, the extension is missing: say so and ask the human to approve
   `gh extension install github/gh-stack` (README requires gh and Git 2.36+; the GitHub tutorial says gh 2.90+).
   Optionally `gh skill install github/gh-stack` installs GitHub's own agent skill; ours does not depend on it.
2. A **layer plan** exists (Story Contract `layer_plan[]`: ordered layers, scope, scenarios per layer, size).
   No plan: stop and ask for one (`ai-mode amigos`, or architect). Do not invent the stack while coding.
3. Clean working tree on the intended base. Set `GH_STACK_NO_UPDATE_NOTIFIER=1` so update notices do not pollute output.

## Actions (one `gh stack` command each)

| Action | Command | Gate |
| --- | --- | --- |
| Start the stack at the bottom layer | `gh stack init <bottom-branch>` (add `--base <trunk>` only if not the default branch) | local |
| Add the next layer on top | `gh stack add <next-branch>` (from the top branch) | local |
| Move between layers | `gh stack up [n]`, `down [n]`, `top`, `bottom`, `trunk`, or `gh stack checkout <branch>` | local |
| Show the stack | `gh stack view --short`; `gh stack view --json` for programmatic state (per branch: `isCurrent`, `isMerged`, `isQueued`, `needsRebase`) | read-only |
| Carry a fix upward | `gh stack rebase --upstack` (from the fixed layer); `view --json` shows `needsRebase: true` on layers that need it | local |
| Resolve a rebase conflict | the rebase exits **3** and pauses; resolve files, `git add`, then `gh stack rebase --continue`; or `gh stack rebase --abort` (restores every branch) | local |
| Push branches only | `gh stack push` | **human yes** |
| Open PRs as drafts | `gh stack submit --auto` (add `--open` only if the human asks for ready-for-review) | **human yes** |
| Sync after merges | `gh stack sync` (`--prune` only if asked) | **human yes** |
| Merge | `gh stack merge --yes --squash` (or the method the human names) | **human yes** |
| Stop tracking a stack locally | `gh stack unstack --local` (touches nothing on GitHub) | local |
| Remove a stack on GitHub | `gh stack unstack` (without `--local`) | **human yes** |

Commit with plain `git add` / `git commit` on the layer's branch; use `gh stack add` only to create the next
branch (its `-m` commit shortcut is not used here, so what lands where stays explicit).

## Branch names

`<story-slug>-<NN>-<layer-slug>`, with `NN` the layer's position in the plan (`01` is the bottom), for example
`avatar-upload-01-data-model`, `avatar-upload-02-endpoints`. Hyphens only.

## Per-layer routine

1. **Check out the layer** you are on (`gh stack view --short`). Never build a layer while on another layer's branch.
2. **Tests first:** un-skip only the scenarios the plan assigns to this layer, watch them fail, then write the
   production change that turns them green. Test level for this layer comes from the plan (jev Stage B result).
3. **Stay in scope.** `git diff <branch-below>..HEAD --stat`: the diff must contain only this layer's scope. Anything
   that belongs to a later layer waits for that layer. If the layer is growing past a quick read, stop and ask
   architect whether it should be split (this changes the plan; do not split silently).
4. **Self-review before moving up:** run the layer's tests, the linters and static analysis, and the CI gates the plan
   marks `active`. Fix problems in this layer. The bottom layer gets the closest look, since mistakes propagate upward.
5. **Then** `gh stack add <next-branch>` and repeat. Do not start the next layer with failing tests below it.

## Fixing feedback

Fix a problem **in the layer that owns it**: `gh stack checkout <that-branch>`, commit the fix there, run that layer's
checks, then `gh stack rebase --upstack` and re-run the checks on each layer above. Never patch a lower layer's problem
from a higher branch. If the cause is a wrong or missing rule in the Story Contract, stop: that reopens the base layer
and architect/product decide, then everything above is restacked.

## Landing

Ask the human, in this order, one at a time: push/submit (drafts), then ready-for-review, then merge. Reviews go bottom-up;
merging goes bottom-up (GitHub retargets the next layer). Report each PR's state from `gh stack view --short` after each step.

## Reporting

After every action say: which layer, what changed (files, scenarios now green), what checks ran and their result, and
what you need from the human (an approval, a decision). If a command fails, quote the error; do not retry blindly or
fall back to raw `git push` / `gh pr create` to get around a gate.

## Notes

- Stack metadata lives in `.git/gh-stack` (not committed). Mutating `gh stack` commands are serialized across the clone;
  linked git worktrees share one catalog, and a branch checked out in another worktree cannot be switched to from here.
- A rebase conflict pauses the stack (exit 3). Any other mutating command is refused with "Run `gh stack rebase --continue` or `--abort` before another mutation". Finish or abort first.
- Verified in a scratch repo on gh-stack v0.2.1: `init` (default branch detected without `--base`), `add`, `view --short/--json`, `checkout`, `rebase --upstack` carrying a lower-layer fix through two layers, conflict pause, `--continue`, `--abort`, `push` to a local remote, `unstack --local`. Not exercised: `submit`, `merge`, `sync` against real GitHub.
- This skill never edits the Story Contract. Changes to scope or the layer plan go back through `ai-mode amigos` or architect.
