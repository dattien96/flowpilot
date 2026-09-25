# BUG-469: Stale flow mirror drops artifactBindings — task_slicer slices the newest CP on disk, not the run's CP

- status: done
- found: live run-96489 → task_slicer child run-96970 (fp-beds/full, BUG-468 live-verify leg)
- fixed_by: CA-966
- tests: internal/runner/bug469_vibe_slicer_source_cp_test.go

## Symptom (live)

A vibe-cp-ingest run that pinned `vibe_cp_doc_id: CP-01` spawned its
task_slicer delegate with an empty generic handoff —
`[flow-engine] Review this result from node "cp_lock"...` — and **no bound
input section at all**. The agent's own template then fell back to "find the
newest `requirements/07-Coding-Plan/todo/CP-*.md`", which on the shared bed
was `CP-02-*.md`. The slicer wrote `Task-15/16/17` with
`Parent Documents: CP-02` while the run had ingested and locked **CP-01** —
wrong-source slicing even though the run's CP identity was pinned correctly.

## Root cause (three layers)

1. **Mirror drops bindings.** `recordFromWorkflowRow` reconstructs
   `ArtifactBindings` only from `step_artifact_bindings` rows joined with
   `artifact_instances`. The live mirror rows had no binding rows at all, so
   every resolved node carried `ArtifactBindings: null` even though the
   embedded pack YAML declares them.
2. **Seed coverage gap.** `builtinHarnessArtifactFlowIDs` only listed
   `task-harness`, `cp-harness`, `cp-harness-smoke` — `vibe-ingest`,
   `vibe-cp-ingest`, `vibe-sprint`, and `bug-plan-harness` declare
   `file_artifact` bindings in the pack but were never seeded. And
   `EnsureBuiltinArtifactBindingsWithStore` only iterates `synced` records
   (flows SyncBuiltins re-upserted this boot), so a mirror already "fresh" by
   pack hash/version but predating the seed never heals.
3. **Templated inputs resolve by "newest match".**
   `appendTemplatedInputArtifactMention` can only tell the node to glob for
   the newest file matching `pathTemplate` — there is no per-run
   instance→concrete-path binding, so even with bindings restored the
   slicer had no way to know *which* CP-*.md was its run's source.

## Fix

- `supabase_workflow_flow_store.go`: `recordFromWorkflowRow` restores a
  builtin node's `ArtifactBindings` from the embedded pack when the mirror
  has none — same precedence rule as the BUG-458 run/posture/model restore
  (pack is authoritative for builtins; nothing admin-editable overrides).
  Heals every builtin flow, including mirrors that never get re-synced.
- `builtin_artifact_bindings.go`: `builtinHarnessArtifactFlowIDs` now covers
  every pack flow declaring bindings (adds `vibe-ingest`, `vibe-cp-ingest`,
  `vibe-sprint`, `bug-plan-harness`); `tdd_signatures` instance UUID added.
  New `EnsureAllBuiltinArtifactBindingsWithStore` seeds every
  binding-bearing builtin unconditionally at startup — heals mirrors the
  synced-only pass skipped.
- `cli/root.go`: calls the heal-all seed after the synced-only seed.
- `artifact_type_registry.go`: `appendResolvedVibeTemplatedInputs` — when a
  node has templated INPUT bindings and the run pinned a source document,
  the prompt names the exact resolved file per slot instead of only the
  "find the newest matching" mention.
- `vibe_cp.go`: `vibeResolvedSlicerSource` returns `rs.vibeLockedCP` (pinned
  at cp-ingest admission, cp_lock approval, and now by
  `forceStartVibeTaskSlicer`) for Vibe runs only; `flow_executor.go`,
  `flow_validate_audit_dispatch.go`, `interactive_service.go` apply the
  resolved-input append at every delegate-spawn/retry site a vibe node can
  reach. No-op for non-vibe runs and nodes without templated inputs.

## Regression cover

- `TestBUG469_SlicerPromptNamesRunsLockedCPNotNewest` — resolved source
  lands in the prompt; no "newest" instruction survives.
- `TestBUG469_SlicerPromptNoResolvedSourceLeavesTemplate` — empty resolved
  path appends nothing (harness/unpinned runs unchanged).
- `TestBUG469_MirrorRowRestoresArtifactBindingsFromPack` — stale mirror row
  (no step_artifact_bindings) reconstructs the slicer with its cp_md input
  binding restored from the embedded pack.
- `TestBUG469_SeedMapCoversAllBuiltinFlowsWithBindings` — every pack flow
  declaring bindings is in the seed map (guards future flows too).
- `TestBUG469_AllBuiltinBindingsSeedHealsUnchangedMirror` — heal-all seed
  POSTs once per binding-bearing builtin + context flow.
