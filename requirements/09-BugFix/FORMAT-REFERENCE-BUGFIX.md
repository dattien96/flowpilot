# BugFix Format Reference

## Metadata

- Document ID: `BUG-XXX`
- Title: `<short bug title>`
- Phase: `bugfix`
- Status: `draft | in_progress | done | superseded | cancelled`
- Owner: `<name or team>`
- Reviewers: `<name or team>`
- Created: `YYYY-MM-DD`
- Last Updated: `YYYY-MM-DD`
- Parent Documents: `<linked CP, SD, SS docs if known>`
- Child Documents: `<optional>`
- Related Documents: `<incident, task, PR, or audit docs>`
- Replaces: `<optional prior bug doc id>`
- Tags: `<feature, module, regression, severity>`

## AI Quick View

### Summary

- `<2 to 4 bullets describing the issue>`

### Current Ask

- `<what needs to be corrected>`

### Key Decisions

- `V-1` `<validation or correction rule>`

### Constraints

- `<risk, rollback, environment, or release constraints>`

### Open Questions

- `<unknowns>`

### Source Refs

- `<run ids, artifact ids, upstream docs, logs>`

## 1. Issue Summary

Describe the observed bug in plain language.

## 2. Parent Links

- impacted coding plan:
- impacted tech design:
- impacted system spec:

## 3. Environment and Reproduction

- environment:
- reproduction steps:
- frequency:

## 4. Expected vs Actual

- expected:
- actual:

## 5. Impact

- users affected:
- workflows affected:
- severity:

## 6. Root Cause

- hypothesis:
- confirmed cause:
- evidence:

## 7. Fix Strategy

- `F-1` `<fix>`
- `F-2` `<fix>`

## 7a. Code Guide Signatures

Exact production signatures the fix must land (name, args, returns, error contract), one fenced block per file, in the file's real language. Mark mechanical-only edits `— unchanged`.

```go
// <path/to/file.go>
func Name(ctx context.Context, a Type) (Out, error) // F-n
```

## 8. Validation

- `V-1` `<verification>`
- `V-2` `<verification>`

## 8a. Test Signatures

Named tests that must exist and pass. Cover the safe-fix-contract matrix: reported repro + near-miss shapes + degraded inputs + ordering/lifecycle + provider parity where shared. Prefer new `bugNNN_*_test.go` / `runNNNN_*_test.go` files — never edit old tests.

- `TestBugXXX_Repro` — red before fix, green after; asserts `<invariant>`
- `TestBugXXX_NearMiss_*` — `<shape>` (covers the same-bug-different-shape class)

## 9. Regression Guard

- tests:
- alerts:
- audit checks:

## 10. Definition of Done

- [ ] Root cause confirmed with evidence (§6) — not a guess
- [ ] §7a signatures landed; §8a tests exist, green, additive-only
- [ ] Every pre-existing test untouched and green; old failure → STOP + report, never edit-to-green (oracle-rule)
- [ ] Claude / Codex / Grok parity proven or tested for shared paths (cross-provider-parity)
- [ ] Prior CA claims for this feature_key not undone
- [ ] CA ledger entry written under the correct feature_key

## 11. Follow-Up Document Updates

- upstream docs that must change:
- notes left unchanged on purpose:
