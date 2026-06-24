# BUG-141: HasBugFixDoc Matches FORMAT-REFERENCE Template

## Metadata

- Document ID: `BUG-141`
- Title: `HasBugFixDoc Matches FORMAT-REFERENCE Template`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: none
- Related Documents: [Task-112: Req Scaffold On Bind](../../08-Task/done/Task-112-Req-Scaffold-On-Bind.md), [BUG-139](./BUG-139-r-bug-Action-Should-Be-Reprompt-Not-Block.md)
- Replaces: none
- Tags: `context-regression-engine, flow-gate, r-bug, reqscaffold, regression`

## AI Quick View

### Summary

- `HasBugFixDoc` checks `strings.Contains(f.Path, "requirements/09-BugFix")`. Task-112's reqscaffold creates `requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md` in every bound project as an untracked file.
- `ObserveGitDiffSince` includes untracked files (via `git status --porcelain -uall`). The template file's path matches the `HasBugFixDoc` predicate, causing it to return `true` even when no real BugFix document was authored.
- Result: r-bug silently does not fire on bug-fix turns in any project that was bound after Task-112 shipped.

### Current Ask

- Exclude `FORMAT-REFERENCE-*.md` files from the `HasBugFixDoc` check.

### Key Decisions

- `D-1` Skip any diff entry whose path contains `"FORMAT-REFERENCE-"` before testing the `requirements/09-BugFix` / `BUG-` predicates.
- `D-2` The same guard does NOT need to be applied to `HasChangeAuditNote` — that function checks for `change-audit/CA-` which no FORMAT-REFERENCE file matches.
- `D-3` The fix is a one-line `continue` guard in `HasBugFixDoc`; no change to `IsDocOrAuditFile` or the rule trigger is needed.

### Constraints

- Must not break the existing `TestHasBugFixDoc` case (a real `BUG-001.md` must still return true).
- Must not affect `HasChangeAuditNote` or `HasCodeChanges`.

### Open Questions

- none

### Source Refs

- Code: `apps/local-runner/internal/flowgate/observe.go` (`HasBugFixDoc`), `apps/local-runner/internal/flowgate/flowgate_test.go`.

## 1. Issue Summary

After Task-112 shipped the `reqscaffold` package, binding any project causes `requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md` to appear as an untracked file in the bound project. `ObserveGitDiffSince` reports untracked files as `Added`, so the template appears in every turn's diff. `HasBugFixDoc` matched it on `strings.Contains(f.Path, "requirements/09-BugFix")`, returning `true` and preventing r-bug from ever firing.

## 2. Parent Links

- impacted coding plan: CP-35 (E2E-11, r-bug rule)
- impacted tech design: SD-20 §2.2 (r-bug), §1 (gate observe)
- impacted system spec: SS-14 (AC-11 force required outputs)

## 3. Environment and Reproduction

- environment: Any project bound after Task-112 was deployed (reqscaffold runs on bind)
- reproduction steps: (1) Bind a fresh project. (2) Ask the AI to fix a bug and say "bug fix" in its final message, without creating a BUG doc. (3) Gate does not fire r-bug.
- frequency: 100% after Task-112 bind

## 4. Expected vs Actual

- expected: r-bug fires and the gate reprompts the AI to create a `BUG-NNN.md` document.
- actual: Gate finds `requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md` in the diff, `HasBugFixDoc` returns true, r-bug does not fire, turn completes silently.

## 5. Impact

- users affected: All users who bound a project after Task-112
- workflows affected: r-bug enforcement across all bound projects
- severity: High — the core r-bug enforcement was completely disabled

## 6. Root Cause

- confirmed cause: `HasBugFixDoc` path predicate `strings.Contains(f.Path, "requirements/09-BugFix")` matches the scaffold template `requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md`, which `ObserveGitDiffSince` includes as an untracked `Added` file.
- evidence: Reproduce by binding a project, running an E2E-11 turn — gate does not fire.

## 7. Fix Strategy

- `F-1` In `HasBugFixDoc` (observe.go), skip any `ChangedFile` whose path contains `"FORMAT-REFERENCE-"` before testing `requirements/09-BugFix` or `BUG-` predicates. The skip applies before both checks so a hypothetically named `FORMAT-REFERENCE-BUG-something.md` is also excluded.

## 8. Validation

- `V-1` `TestHasBugFixDocIgnoresFormatReferenceFile` — diff with only FORMAT-REFERENCE-BUGFIX.md + calc.go → false. ✓
- `V-2` `TestHasBugFixDocRealDocAlongsideFormatReference` — real BUG doc alongside FORMAT-REFERENCE → true. ✓
- `V-3` `TestHasBugFixDoc` (existing) — real BUG-001.md → still true. ✓
- `V-4` Manual E2E-11 after runner rebuild: gate reprompts when no BUG doc written. ⏳

## 9. Regression Guard

- tests: `flowgate_test.go` `TestHasBugFixDocIgnoresFormatReferenceFile` and `TestHasBugFixDocRealDocAlongsideFormatReference`.
- audit checks: Confirm `HasChangeAuditNote` is unaffected (it uses `change-audit/CA-` predicate).

## 10. Follow-Up Document Updates

- SD-20 §2.2: no change needed — the fix is a predicate refinement, not a semantic rule change.
- Task-112: the root cause was a consequence of Task-112; the fix is in the flowgate observer, not in reqscaffold itself. The scaffold behavior (creating FORMAT-REFERENCE-BUGFIX.md) is correct and should be kept.
