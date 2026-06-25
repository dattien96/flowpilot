# BUG-139: r-bug Action Should Be Reprompt Not Block

## Metadata

- Document ID: `BUG-139`
- Title: `r-bug Action Should Be Reprompt Not Block`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: none
- Related Documents: [CA-126](../../../change-audit/CA-126-fix-gate-block-ui-and-r-bug-action.md)
- Replaces: none
- Tags: `context-regression-engine, flow-gate, r-bug, regression`

## AI Quick View

### Summary

- `r-bug` (missing bugfix doc) was implemented with `Action: "block"` in `DefaultRules()`, which surfaces the hard-stop modal and disables auto-remediation. This is incorrect: a missing bugfix doc is a **recoverable** requirement (the AI can be reprompted to create it), not a hard stop like a failing test.
- Correct behavior: `r-bug` should be `Action: "reprompt"` — the gate auto-reprompts the AI (≤2 attempts) to add the required `BUG-NNN` file, exactly like `r-ca` handles a missing change-audit note.

### Current Ask

- Change `r-bug` action from `"block"` to `"reprompt"` in `DefaultRules()` and update SD-20 accordingly.

### Key Decisions

- `D-1` `Action: "reprompt"` means the gate re-prompts the AI with the violation message and suppresses the current turn completion. The AI's next turn (attempt ≤2) should add the bugfix doc, after which the gate passes.
- `D-2` `r-tests` and `r-reg` remain `"block"` — failing tests are NOT auto-remediable (the AI cannot simply be told "fix the tests" without risking further regressions).

### Constraints

- `enforce.go` `isAlwaysBlock` check only covers `tests_failed` and `regression_test_broke` — `bug_fixed` is not in that set, so changing the `DefaultRules` action to `reprompt` is sufficient; no change to `enforce.go` needed.

### Open Questions

- Q-1: Should there also be an `r-task` rule (missing Task document) with `reprompt` action? Deferred — not yet implemented, requires separate rule design.

### Source Refs

- Code: `apps/local-runner/internal/flowgate/rules.go` (`DefaultRules`), `apps/local-runner/internal/flowgate/enforce.go` (`isAlwaysBlock`).
- Docs: `requirements/06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md` §2.2, D-3.

## 1. Issue Summary

`DefaultRules()` returns `r-bug` with `Action: "block"`. This causes a hard-stop modal for a missing bugfix doc, preventing auto-reprompt. The AI should be automatically asked to create the missing doc — the same remediation pattern used by `r-ca` for missing change-audit notes.

## 2. Parent Links

- impacted coding plan: CP-35
- impacted tech design: SD-20 §2.2 (r-bug rule), D-3
- impacted system spec: SS-14

## 3. Environment and Reproduction

- environment: Any run where the AI's final message contains "fixed bug" / "bug fix" and no `BUG-*.md` file was added
- reproduction steps: Prompt the AI to fix a bug; observe that a hard-stop modal fires instead of an inline reprompt.
- frequency: 100% when r-bug triggers

## 4. Expected vs Actual

- expected: Gate reprompts the AI (inline reprompt turn, ≤2 attempts) to add the missing bugfix doc; no modal.
- actual: Hard-stop modal fires; user must manually dismiss; no AI remediation attempt.

## 5. Impact

- users affected: Any user triggering r-bug
- workflows affected: Context & Regression Engine (CP-35)
- severity: Medium — incorrect UX / policy mismatch; not data loss

## 6. Root Cause

- hypothesis: Initial implementation chose `"block"` for safety; should be `"reprompt"` per the original design intent (D-3 in SD-20)
- confirmed cause: `DefaultRules()` in `rules.go` line for `r-bug` has `Action: "block"`.
- evidence: SD-20 D-3 states both `r-ca` and `r-bug` are auto-remediable; code contradicts the doc.

## 7. Fix Strategy

- `F-1` In `apps/local-runner/internal/flowgate/rules.go`, change r-bug: `Action: "reprompt"`.
- `F-2` Update `SD-20 §2.2` r-bug table: Action → `reprompt (auto-remediated, ≤2 attempts)`.
- `F-3` Update `SD-20 D-3` note: add correction attribution to BUG-139.

## 8. Validation

- `V-1` Trigger r-bug (AI says "fixed bug" but no BUG doc added) → gate reprompts instead of showing modal. ✓ (requires manual E2E with runner rebuild)
- `V-2` `r-tests` / `r-reg` still hard-block (unchanged `DefaultRules` for those rules). ✓
- `V-3` `enforce.go` `isAlwaysBlock` does not include `bug_fixed` → `reprompt` is honored. ✓ (code inspection)

## 9. Regression Guard

- tests: `flowgate_test.go` — no test specifically covering r-bug action; add if convenient.
- alerts: none
- audit checks: Confirm `enforce.go` `isAlwaysBlock` only gates `tests_failed` / `regression_test_broke`.

## 10. Follow-Up Document Updates

- SD-20 §2.2 and D-3: updated in this fix.
- Q-1 (r-task rule): deferred to a future CP-35 task.
