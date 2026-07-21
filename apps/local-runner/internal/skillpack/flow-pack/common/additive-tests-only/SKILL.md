---
name: additive-tests-only
description: When fixing bugs or implementing tasks, only add new tests. Never edit pre-existing tests without stopping to ask the user first — green old tests are the regression guard.
version: 6
---

# additive-tests-only

Use **PROACTIVELY** on every bugfix, task implementation, and regression fix that touches production code.

Companion to **oracle-rule** (do not weaken tests to pass). This skill covers the complementary contract: **do not touch the legacy suite at all** unless the human approves.

## The Rule

> When fixing a bug or shipping a task: **only write new tests**.  
> If a pre-existing test must be changed for your work to compile or pass, **stop and ask the user before editing it**.

Green old tests (unchanged) are the primary signal that the change did not regress prior behavior. Editing them silently defeats that signal.

## Allowed

1. **Add** new test files (preferred) or new `Test*` / cases in a **new** dedicated file (e.g. `run5296_skip_prose_escalate_test.go`).
2. **Add** new subtests / helpers **only inside new files** when practical.
3. Run **filtered** suites that include both new tests and related old patterns — report pass/fail; do not “fix” old failures by editing them.
4. If production **API signatures** change and old tests fail to compile, **do not** mass-edit them: stop, explain the conflict, ask the user whether to:
   - keep a compatibility shim so old tests stay untouched, or
   - allow a scoped update to old tests with explicit approval.

## Forbidden without user approval

1. Edit assertions, expected values, fixtures, or timeouts in **pre-existing** tests.
2. Rename, skip, delete, or comment out legacy tests to green CI.
3. “Drive-by” cleanup of old tests while on a bug/task.
4. Change shared test helpers in a way that alters behavior of existing tests (that counts as editing the old suite).

## When an old test fails or will not compile

1. **Stop.** Do not patch the test.
2. Diagnose: production regression vs intentional behavior change vs API break.
3. Prefer: fix production code, or add a thin compatibility path so old tests keep working unchanged.
4. If the old test is truly wrong vs current AC/spec: **ask the user**, cite the test path + AC/spec, wait for yes before any edit.
5. After approval, keep the edit **minimal** and record it in the change-audit note.

## Workflow (bug / task)

```text
1. Implement production fix
2. Add NEW regression test(s) only (new file preferred)
3. Run: new tests + relevant old patterns (no full suite required)
4. Old tests all green and untouched → done
5. Old test red or needs edit → STOP → ask user
```

## How this prevents regression

| Signal | Meaning |
|--------|---------|
| Old tests unchanged + still green | Prior contracts still hold |
| New tests green | New bug/task behavior locked in |
| Old tests edited without review | Regression signal lost |

Full-package green is optional; **zero unsolicited edits to the legacy suite** is mandatory.

## Relationship to oracle-rule

| Skill | Focus |
|-------|--------|
| **oracle-rule** | Failing test → fix **code**, not the assertion (unless user agrees test is wrong) |
| **additive-tests-only** | New work → **add** tests only; do not modify the legacy suite without asking |

Both apply together on every bugfix and coding task.
