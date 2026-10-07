# QA Testing Guidelines (vendored)

Reference guides the **qa** role loads on demand, one layer at a time, to keep
within its context budget (`[qa]` ctx-size = 65536). See the decision table in
[`../../qa.md`](../../qa.md).

## Index

| Test type | Guide |
| --- | --- |
| Unit | [`unit.md`](unit.md) |
| Functional | [`functional.md`](functional.md) |
| Integration | [`integration.md`](integration.md) |
| Component | [`component.md`](component.md) |
| Contract | [`contract.md`](contract.md) |
| E2E | [`e2e.md`](e2e.md) |
| Visual regression | [`visual-regression.md`](visual-regression.md) |
| Mutation | [`mutation.md`](mutation.md) |
| AI skill / eval | [`ai-skill.md`](ai-skill.md) |

## Source of truth

**This vendored copy is canonical for the repo.** It is a committed snapshot so
`ai-mode` stays portable (no dependency on any machine-local path). The upstream
author copy lives outside the repo.

## Re-sync after upstream edits

Re-vendor from the upstream "The Bible" working copy with:

```bash
SRC="$HOME/Desktop/The Bible"; DST="presets/agents/qa/guidelines"
cp "$SRC/Unit Testing Guidelines.md"              "$DST/unit.md"
cp "$SRC/Functional Testing Guidelines.md"        "$DST/functional.md"
cp "$SRC/Integration Testing Guidelines.md"       "$DST/integration.md"
cp "$SRC/Component Testing Guidelines.md"          "$DST/component.md"
cp "$SRC/Contract Testing Guidelines.md"          "$DST/contract.md"
cp "$SRC/E2E Testing Guidelines.md"               "$DST/e2e.md"
cp "$SRC/Visual Regression Testing Guidelines.md" "$DST/visual-regression.md"
cp "$SRC/Mutation Testing Guidelines.md"          "$DST/mutation.md"
cp "$SRC/AI Skill Testing Guidelines.md"          "$DST/ai-skill.md"
```
