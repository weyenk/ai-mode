# Mutation Testing Guidelines

*Maintained by the Engineering Quality Team. Last reviewed June 2026.*

**Related pages:** These guidelines sit alongside the [Unit Testing Guidelines](Unit Testing Guidelines.md) — read that page first if you are new to the test strategy. For broader test-pyramid context, see the **Quality in Product Engineering** parent page.

## 1. Purpose and scope

**Mutation testing** evaluates the *quality* of your unit tests, not just whether code runs. A mutation tool injects small, intentional changes (mutants) into production code—flipping a boundary operator, removing a conditional, changing a return value—and re-runs your unit tests. If a test fails, the mutant is **killed**. If all tests still pass, the mutant **survived**, which usually means your tests do not assert strongly enough on that behavior.

Mutation testing is a **unit-test quality signal**. It does not replace integration, functional, or E2E tests. High line coverage can still leave weak assertions; mutation testing catches tests that execute code without verifying outcomes.

Use mutation testing when you want confidence that unit tests would catch realistic bugs—not when you need to validate cross-service contracts or user flows.

## 2. When to run mutation testing

| Trigger | Run target | Notes |
| --- | --- | --- |
| **PR / feature branch** | Changed files in scope only | Default for developers; fast feedback |
| **Pre-merge gate (critical modules)** | Entire module or package | Auth, pricing, payments, bet placement, PII |
| **Nightly / scheduled** | Configured module set | Full scope; trend mutation score over time |
| **Ad hoc / skill invocation** | User-defined scope | Via the `create-mutation-tests` Cursor skill |

**Prerequisite:** The unit test suite must be **green** before a mutation run. Do not run mutation testing on a failing suite—the results are meaningless.

## 3. Approved tooling

Use the mutation tool already configured in the repo. Do not introduce a second mutation framework without team agreement.

| Stack | Tool | Config location | Typical command |
| --- | --- | --- | --- |
| JS/TS (Jest, Vitest, Mocha) | [Stryker](https://stryker-mutator.io/docs/) | `stryker.config.*` | `npm run test:mutation` |
| Java/Kotlin | [PIT](https://pitest.org/) | `pom.xml` / Gradle | `mvn org.pitest:pitest-maven:mutationCoverage` |
| Python (pytest) | [mutmut](https://mutmut.readthedocs.io/) | `pyproject.toml` / `.mutmut-cache` | `mutmut run` |
| .NET | [Stryker.NET](https://stryker-mutator.io/docs/stryker-net/introduction/) | `stryker-config.json` | `dotnet stryker` |
| Other | Project-adopted tool | Document in repo README | — |

## 4. Scope rules

**Acceptable scope must be explicit:** paths, modules, packages, or a file list. Never run an unbounded whole-monorepo mutation without acknowledging runtime cost.

| Run target | Meaning |
| --- | --- |
| **Changes only** | Mutate files or lines touched in the PR or branch diff. Use incremental/diff mode when the tool supports it. |
| **Entire scope** | Mutate all production code in the named module or package. Apply exclude patterns from §6. |

**Scope inference on feature branches:** Default to `git diff` against the default branch (`main` / `master`). Branch names with a Jira key prefix (e.g. `TOAD-1234-feature-name`) may expand context via the linked story.

## 5. Mutation score thresholds

Team defaults below. Individual repos may tighten thresholds in their mutation config.

| Context | Minimum mutation score | Action if below |
| --- | --- | --- |
| PR (changed files) | **No hard gate (v1)** — report only | Triage Critical and High survivors before merge (team judgment) |
| Critical modules (auth, pricing, payments) | **≥ 70%** | Block merge or require documented exception |
| Nightly trend | No regression \> 5 pts week-over-week | Investigate in next sprint |

Scores are informational on non-critical paths until teams adopt local gates.

**Mutation score** = mutants killed ÷ total mutants in scope (expressed as a percentage).

## 6. What to exclude from mutation

These patterns are **Low triage by default**. Exclude them in mutation config when possible.

| Category | Examples |
| --- | --- |
| Generated code | `**/generated/**`, protobuf/gRPC stubs, OpenAPI clients marked generated |
| DTOs / data-only types | Pure data classes with no logic |
| Logging and debug | Log statements, debug helpers |
| Framework boilerplate | `main`, default `toString`, `hashCode`, `equals` with no custom logic |
| Third-party / vendored | Code not owned by the team |
| Config and build | Config-only files, migrations, build scripts |

**Per-repo config examples:**

- **Stryker:** `mutate` globs, `ignorePatterns`, `excludedMutations`
- **PIT:** `excludedClasses`, `excludedMethods`, `excludedTestClasses`
- **mutmut:** `# pragma: no mutate` on specific lines

**Ignoring is not a free pass.** Each ignore entry needs a one-line rationale in config or the PR description.

## 7. Triage rubric (surviving mutants)

A **survivor** is a mutant your tests did not kill. Assign each survivor one importance level.

| Level | Definition | Examples | Required action |
| --- | --- | --- | --- |
| **Critical** | Security, money, PII, auth, idempotency, concurrency | Conditional removed on auth check; boundary operator flipped on stake or price | **Must** add a test or fix before merge; escalate if product change is needed |
| **High** | Core business rules, error paths, state transitions | Missing assertion on error branch; untested wrong default | Add test in same PR or linked follow-up before merge |
| **Medium** | Secondary logic, non-default branches | Optional validation edge case | Add test or accept with Jira ticket |
| **Low** | Logging, trivial accessors, generated/boilerplate | Matches §6 exclude patterns | Ignore in config or accept with rationale |

### Triage report template

Teams and the `create-mutation-tests` skill use this shape:

```markdown
## Mutation run summary
- Scope: [paths/modules]
- Run target: changes only | entire scope
- Tool / command: [e.g. npm run test:mutation]
- Mutation score: X% (killed/total)
- Guidelines applied: yes | no (reason)

## Surviving mutants (triaged)

### Critical
| File:line | Mutator | Change summary | Why it matters | Recommended action |

### High
...

### Medium
...

### Low
...
```

When strengthening tests, prioritize **Critical → High → Medium**. Skip Low unless explicitly requested.

## 8. When to ignore a surviving mutant

Ignore a survivor (via config + documented rationale) when **all** of the following apply:

1. Triage level is **Low**, OR the code matches §6 exclude patterns
2. Adding a test would assert implementation detail without behavioral value
3. The mutant is equivalent (e.g. string literal mutation in user-facing copy with no logic impact)
4. Cost to kill exceeds value (document in PR; Tech Lead or QE sign-off for Critical/High deferrals)

**Rules:**

- **Never ignore without documentation** — PR comment, Jira link, or config comment.
- **Never ignore Critical or High** without explicit exception approval (QE + dev lead).

## 9. Post-run actions (standard playbook)

| Action | When to use |
| --- | --- |
| **Report only** | Exploratory run, CI nightly review |
| **Strengthen tests** | Default for PR work — add unit tests for Critical/High survivors (test code only) |
| **Configure ignores** | Low survivors matching §6; document rationale |
| **Track externally** | Jira ticket for deferred Medium+ items |
| **Re-run** | After test changes; confirm score improved and survivors killed |

Product or application code changes to kill mutants require **explicit approval**. Prefer test improvements first.

## 10. Using the `create-mutation-tests` Cursor skill

The skill implements this page at runtime. It is the recommended way to run mutation testing locally with consistent triage and post-run handling.

|  |  |
| --- | --- |
| **Invocation** | Explicit only — `@create-mutation-tests` or skill picker. Not auto-applied. |
| **Repo** | `ee-quality-cursor-skills` (your organization's quality Cursor skills repository) — activate via `link-active-skills.sh` |
| **Source of truth** | Loads the canonical guidelines document (e.g. via your team's docs MCP or local copy); falls back to `docs/standards/mutation-testing-guidelines.md` in the skills repo |

**What the skill does:**

1. Prompts for **scope**, **run target** (changes only vs entire scope), and **post-run action** if not supplied
2. Loads the canonical **Mutation Testing Guidelines** document
3. Detects language and existing mutation tooling
4. Runs mutation testing on the scoped area
5. Triages surviving mutants per §7
6. Executes the chosen post-run action per §9

**What the skill does not do:**

- Modify product code without approval
- Skip or fake success when the unit suite is red or tooling is missing
- Invent thresholds not defined on this page

**Related skills:**

- `create-unit-tests` — writing isolated unit tests
- `expose-the-testing-gaps` — gap analysis without running a mutator

**Example invocation:**

> Run mutation testing on `src/pricing/` — changed files only, strengthen tests for Critical and High survivors.

## 11. CI and ownership

| Context | Expectation |
| --- | --- |
| **PR** | Optional mutation step on changed paths; publish HTML or JSON report as a CI artifact |
| **Nightly** | Full module runs for designated services; track score trend |
| **Ownership** | QE defines standards (this page); service teams own config and remediation |
