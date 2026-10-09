# CA-653 — vibe clone role extended to system-rooted clones (CA-652 follow-up)

## Scope
Amendment of CA-652, same surface. A user-owned clone whose ancestry roots
at a **system-only** vibe built-in (vibe-sprint / vibe-owner-debate /
vibe-adopt-sprint) is now startable with user kind in vibe mode too — a
clone keeps its source's role, and the user explicitly asked for their
vibe-sprint copy in the vibe picker. Canonical system ids stay forbidden
exactly as pinned; out-of-scope items from CA-652 unchanged.

## Evidence
After CA-652 the user (dev-mode runner, vibe-mode UI) cloned vibe-sprint
and still saw only the 4 entries: correct per the CA-652 design, which
admitted user roots only. Requirement update: sprint clones must appear.

## Fix
- `CloneInheritsVibeUserStart` → `CloneInheritsVibeStart`
  (`VibeUserAncestorRoot` → `VibeAncestorRoot`); admits `isVibeUser(root)
  || isVibeSystem(root)`.
- `allowVibeCloneUserStart` → `allowVibeCloneStart` (3 call sites renamed).
- TS `vibeCloneIds` admits system roots (no signature change; callers
  untouched).
- Caught by existing suite before commit: the broadened walker admitted a
  **mirror row's own UUID** (first hop already carries a pack identity),
  breaking `TestEnforceWorkingModeStart_VibeWorkflowUUIDSystemFlowForbidden`
  (CA-1078 laundering pin). Fixed with a first-hop guard — only pack-less
  user rows may inherit; mirrors/canonical refs stay owned by the main
  gate. Both new tests pin the guard (mirror UUID → false, both layers).
  TS already had the equivalent `!w.packFlowId` guard.

## Tests
- Renamed/extended unit tests green (incl. mirror-UUID-must-not-inherit).
- Full CA-1078 file green (resolve + both laundering pins + forward pins).
- Gate/mirror/clone regression sweep green except pre-existing
  `TestFlowDefinitionStoreForUnconfiguredRunnerYieldsNil` (env, fails on
  clean main).
- Desktop `tsc --noEmit` clean; `workflowSelect.test.ts` 7/7 via tsx
  (sprint-clone expectations updated to the new requirement).
