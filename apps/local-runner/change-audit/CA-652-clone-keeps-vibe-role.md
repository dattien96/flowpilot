# CA-652 — clone keeps its source's vibe role (clone-of-vibe startable in vibe mode)

## Scope
Narrow additive allowance on the working-mode family gate. A user-owned flow
whose `cloned_from` ancestry roots at a vibe **user-startable** built-in
(vibe-ingest / vibe-cp-ingest / vibe-tasks / vibe-adopt) may start with user
kind in vibe mode — on desktop pickers and the runner gate alike.
Out of scope: chains rooted at system-only flows (vibe-sprint /
vibe-owner-debate / vibe-adopt-sprint) stay rejected like their canonical
ids; `selectableIn`/`editable` untouched; canonical-id pins untouched;
vibe runtime special-casing keyed by canonical id (cp source fence,
sprint chaining, debate overlays) intentionally still keys off canonical
ids — a clone runs as a plain flow graph in vibe mode.

## Evidence
User cloned a vibe flow, set a new model, but the copy never appears in the
Flow-tab Workflow dropdown in vibe mode. Root cause chain, all verified by
direct read:
- Clone rows carry `pack_id/pack_flow_id = NULL`, so their identity is the
  row UUID, which no vibe set contains.
- Desktop `userFlowSelectableForMode("vibe", uuid)` → false
  (`workingMode.ts:96-97`); `filterWorkflowsForWorkingMode` drops the row.
- Runner `FlowAllowedForWorkingMode(vibe, uuid, user)` → forbidden
  (`workingmode.go:180-185`); the armed UUID fails all three user-start
  gates (turn create, forward fences, picker-validate).
- Copying the source's pack keys onto the clone was rejected: mirror sync
  keys mirrors by (pack_id, pack_flow_id), so a clone sharing them risks
  `GetByPackFlow` collisions and user-data overwrite on re-upsert.
  Provenance already exists instead: `workflows.cloned_from`
  (client-core writes it, Go `FlowDefinitionRecord.ClonedFrom` reads it).
- GitNexus impact on `FlowAllowedForWorkingMode`: LOW, 3 direct callers
  (exactly the 3 gate sites below), 0 processes.

## Fix
- `internal/workingmode/vibe_clone_ancestry.go` (new): `FlowAncestryLookup`
  + `VibeUserAncestorRoot` (bounded, cycle-guarded, fail-closed) +
  `CloneInheritsVibeUserStart` (admits only vibe-**user** roots).
- `internal/runner/vibe_clone_gate.go` (new): `allowVibeCloneUserStart`
  (vibe-mode-only, nil-store-safe; `GetByRef` ancestry lookup). Wired as
  fallback-after-reject at the 3 user-start gates, so it can only ever
  admit: `working_mode_http.go` (picker-validate, reuses the CA-1078
  resolve-then-gate shape), `interactive_handlers.go` (turn create),
  `interactive_service.go` (forward fences, lock discipline per the
  existing deadlock note). No extra store reads on any passing path.
- Desktop: `contract.ts` Workflow gains `clonedFrom`;
  `mapNavigatorWorkflow` carries it (CA-1070 precedent);
  `workingMode.ts` gains `vibeCloneIds` + optional set param on
  `userFlowSelectableForMode`/`filterWorkflowsForWorkingMode` (absent set
  fails closed — all existing callers/tests keep exact behavior);
  `ChatWorkspace.tsx` (single memoized set) and `store.ts`
  (selectWorkflow pre-arm, mode-switch selected check, pickerVibeArm)
  pass it through.

## Tests
- `internal/workingmode/vibe_clone_ancestry_test.go` (new): RED (undefined
  symbols) → GREEN. Covers user-root admit, clone-of-clone, system-root
  reject, no-provenance/broken-chain/cycle/unknown/empty/nil fail-closed,
  canonical ids left to the main gate.
- `internal/runner/vibe_clone_gate_test.go` (new, fake store): RED
  (undefined) → GREEN. Covers admit, clone-of-clone, system-root,
  plain/unknown/nil-store/dev-mode reject.
- Desktop `workingMode.workflowSelect.test.ts` (+3 tests): RED (not a
  function) → GREEN, 7/7 file green via tsx.
- `tests/phase1/navigatorCatalog.test.ts`: expectations extended with the
  carried `clonedFrom` (additive, CA-1070 precedent — not weakened).
- `tsc --noEmit` (desktop package): clean.
- Desktop `tsc -p tsconfig.phase1-tests.json`: clean.
- Phase1 pipeline (807 tests): 794 pass; the 13 failures are byte-identical
  (modulo timings) to a clean-main baseline run (804 tests / same 13 fail)
  — all pre-existing, none in vibe/clone/navigator surfaces. All 3 new
  clone-ancestry tests and every ca1070/navigator/cloneWorkflow test pass.
- Full `go test ./internal/runner/...` (2 runs): failure sets differ
  run-to-run (15 vs 19) while the change is fixed — suite-order/timing
  flakes. Union attribution: 12 fail identically on clean main
  (pre-existing env: Firebase e2e, provider session/pool, catalog-fake,
  unconfigured-store); every other name (BUG368, Task447, Bug1194,
  DuplicateFreeze, R6/R7, RecoveredFlow, ResumeRun, VibeSprintBoundary)
  PASSES in isolation with this change applied, and none of their files
  references `Cloneable`. Untouched per additive-tests-only.
