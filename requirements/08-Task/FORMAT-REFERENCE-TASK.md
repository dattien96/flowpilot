# Task Format Reference

## Metadata

- Document ID: `Task-XXX`
- Title: `<short task title>`
- Phase: `task`
- Status: `draft | in_progress | done | superseded | cancelled`
- Owner: `<name or team>`
- Reviewers: `<name or team>`
- Created: `YYYY-MM-DD`
- Last Updated: `YYYY-MM-DD`
- Parent Documents: `<linked CP, SD, SS docs>`
- Child Documents: `<optional>`
- Related Documents: `<optional>`
- Replaces: `<optional prior task doc id>`
- Tags: `<feature, module, domain>`

## AI Quick View

### Summary

- `<2 to 4 bullets describing this specific slice>`

### Current Ask

- `<what should be done now>`

### Key Decisions

- `T-1` `<task-specific rule>`

### Constraints

- `<do not touch, deadline, compatibility, or sequencing constraints>`

### Open Questions

- `<unknowns>`

### Source Refs

- `<CP ids, P ids, SD ids, SS ids>`

## 1. Goal

Describe the concrete outcome of this task.

## 2. Parent Links

- coding plan:
- tech design:
- system spec:
- specific upstream ids:

## 3. Trigger

Explain why this task exists now.

## 4. Exact Change

- `T-1` `<change>`
- `T-2` `<change>`

## 5. Touched Areas

- files:
- modules:
- routes:
- tables:

## 6. Code Guide Signatures

Exact production signatures the implementer must land (name, args, returns, error contract). One fenced block per file. Use the real language of the file (Go / TS). Leave a `— unchanged` note for files touched only mechanically.

```go
// <path/to/file.go>
func Name(ctx context.Context, a Type, b Type) (Out, error) // T-n
```

```ts
// <path/to/file.ts>
export function name(a: Type, b: Type): Out // T-n
```

## 7. Test Signatures

Named tests that must exist and pass — this is the executable half of DoD. Each line maps to an AC: `TestXxx` (Go) or `test("...")` (vitest). Cover the safe-fix-contract matrix: reported path + near-miss shapes + degraded inputs + ordering/lifecycle + provider parity where shared.

- `TestXxx_Scenario` — asserts `<invariant>` (covers AC-n)
- `test("xxx", ...)` — asserts `<invariant>` (covers AC-n)

## 8. Acceptance Check

- `<what must be verified after implementation — behavior-level, beyond tests>`

## 9. Out of Scope

- `<what this task must not expand into>`

## 10. Definition of Done

- [ ] All §6 signatures implemented exactly (or deviation documented in §11)
- [ ] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [ ] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [ ] Provider parity proven or evidenced where the change touches shared/provider paths (R2)
- [ ] `feature_key` set; CA ledger entry written; FEATURE-KEYS.md already contains the key
- [ ] §8 acceptance checks verified by hand or test
- [ ] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
