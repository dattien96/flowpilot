# CA-655 — CP-doc picker follows clone ancestry (CA-652/654 follow-up)

## Scope
UI-only. A vibe-tasks/cp-ingest clone selected in the Workflow tab or the
armed bar showed no "Select CP doc" picker (both regex gates keyed off bare
pack ids; a clone UUID never matches). Server CP-source fences
intentionally untouched — they key off canonical ids; the UI now forces a
doc pick (Start stays disabled until chosen), and reader nodes fail closed
downstream.

## Fix
- `workingMode.ts`: new `vibeSourceRootId(workflows, ref)` — bare/pack
  ids pass through via `bareFlowId` (byte-identical to the old direct
  regex for every static/unknown ref); mirror rows yield their pack id;
  clone UUIDs walk `cloned_from` to the root pack id; unresolvable clone
  chains yield "" (no picker).
- `ChatInput.tsx`: `armedNeedsSourceDoc` (CP options loading) and
  `needsSourceDoc` (picker + Start gating) test the resolved root.
- Side effect (intended): a mirror row armed by raw UUID now also resolves
  to its pack id for this gate — previously silent no-picker; strictly
  more correct, same direction as CA-1078.

## Tests
- `workingMode.workflowSelect.test.ts` (+1 test): RED (not defined) →
  GREEN; 13/13 across both workingMode test files via tsx.
- `tsc --noEmit` (desktop package): clean.
- `detect-changes`: HIGH via ChatInput centrality (coarse symbol; the diff
  is 3 additive lines, no logic restructuring) — accepted with the above
  evidence.
