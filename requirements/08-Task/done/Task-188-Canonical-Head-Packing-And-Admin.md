# Task-188: Canonical-Head Packing And Admin Visibility

## Metadata

- Document ID: `Task-188`
- Title: `Canonical-Head Packing And Admin Visibility`
- Phase: `task`
- Status: `done` (2026-08-11 — code complete per [CA-435](../../change-audit/CA-435-cp43-p5-pack-budget-and-admin-scope-diff.md); manual B19/B20 = tick in [CP-43-Test-Steps](../../07-Coding-Plan/done/CP-43-Test-Steps.md))
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-03`
- Last Updated: `2026-08-11`
- Parent Documents: [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/done/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (P-5), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (AC-3, AC-8, AC-15)
- Child Documents: `None`
- Related Documents: [Task-186: Canonical Head And Intent Signature](./Task-186-Canonical-Head-And-Intent-Signature.md), [Task-187: Superseding Decision Records And Retire](./Task-187-Superseding-Decision-Records-And-Retire.md), [Task-097: Feature Catalog And Resolver](../../08-Task/done/Task-097-Feature-Catalog-And-Resolver.md), [Task-103: Engine Local Store And Drive Sync](../../08-Task/done/Task-103-Engine-Local-Store-And-Drive-Sync.md)
- Replaces: `None`
- Tags: `prompt-packing, contextresolver, contextsync, desktop-flowpilot, canonical-head, local-runner`

## AI Quick View

### Summary

- Make the prompt lead with the Canonical Head (behavior + signature status + rejected decisions) **before** any ordered history; demote raw churn to a lower-priority budget item.
- Sync `canonical/*.json` via the CP-35 `context-engine/` Drive mechanism; keep `contracts.ndjson` local-only.
- Add a "Canonical Head" panel in the desktop app (`apps/desktop-flowpilot` — the only active UI target; `apps/admin-web` is deprecated/dropped and must not be touched): behavior, signature-status chip, rejected-decisions list, and the active step's in/out-of-scope diff.

### Current Ask

- Implement `SD-21 P-5`: Head-first packing extension, contextsync shared-set update, and a desktop-app panel.

### Key Decisions

- `T-1` Head is a mandatory packing slot; raw history is optional/lower priority (`SD-21 D-3`).
- `T-2` `spec_less` Heads render a low-confidence flag in the packed block.

### Constraints

- Extend CP-35 §4.2.1 `feature.history` packing; reuse CP-35 P-8 `contextsync` Drive sync; do not add DB tables.
- Non-fatal: a packing/sync failure degrades to prior behavior (`AC-9`).

### Open Questions

- Budget threshold at which raw history is dropped entirely vs summarized (ties to CP-23/CP-10 §5 packer).

### Source Refs

- `SD-21 §4`, `§6` (packing/sync contracts), `§7` step 2/8, `§13.3`. `CP-43 §4.5`. `SS-14 AC-3`, `AC-8`, `AC-15`.

## 1. Goal

On a resolved code turn, the AI reads the Canonical Head first (current truth + rejected dead-ends), not the chaotic ordered log; operators can inspect Head status and step scope in the desktop app's Projects settings.

## 2. Parent Links

- coding plan: `CP-43` P-5
- tech design: `SD-21` §6 (packing + sync), D-3
- system spec: `SS-14` AC-3, AC-8, AC-15
- specific upstream ids: `P-5`, `AC-3`, `AC-8`

## 3. Trigger

Task-186/187 produce the Head + decisions, but they only help if the AI reads the Head *before* the log. This task wires the Head into prompt assembly and exposes it for human inspection.

## 4. Exact Change

- `T-1` `featurecatalog`/`contextresolver` `feature.history` slot — prepend a Canonical Head block: `## Canonical state of "<feature>"` with `behavior_statement`, `intent_signature` short + `status` chip, and a "do NOT re-attempt" list from `decisions`; then the ordered history as a lower-priority section.
- `T-2` Budget: mark the Head block mandatory; the raw history block is lower priority in the packer (CP-23/CP-10 §5) and may be dropped/summarized under budget pressure — `log()` when dropped.
- `T-3` `spec_less` rendering — annotate the block "(spec-less — low confidence)" when `spec_confidence="spec_less"`.
- `T-4` `contextsync` (CP-35 P-8) — add `canonical/*.json` to the shared/Drive-synced set under `context-engine/`; assert `contracts.ndjson` stays local-only.
- `T-5` Desktop app "Canonical Head" panel (per feature, `apps/desktop-flowpilot`): behavior statement, signature-status chip (`current`/`spec_drifted`/`code_drifted`/`spec_less`/retired), rejected-decisions list, and the active step's `Contract` with in-scope/out-of-scope path highlighting.
- `T-6` Tests — packing unit: Head prepended before history, `spec_less` flag present, decisions rendered; drop-history-under-budget path logs the drop. Integration: prompt leads with Head across Claude/Codex. Sync: `canonical/*.json` appears in the `context-engine/` manifest; `contracts.ndjson` never syncs.

## 5. Touched Areas

- files: `apps/local-runner/internal/featurecatalog/` (history slot), `internal/contextsync/` (shared set), `apps/local-runner/internal/runner/canonical_head_handlers.go` (read endpoints), `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx` (Canonical Head panel)
- modules: `featurecatalog`, `contextresolver`, `contextsync`, desktop-flowpilot
- routes: engine read endpoints for Head/contract (reuse existing engine API surface)
- tables: none (local JSON + Drive manifest)

## 6. Acceptance Check (DoD)

- [x] The `feature.history` slot prepends the Canonical Head block before any ordered history on a resolved code turn. Implemented in `featureHistorySource.Fetch` (`context_sources_builtin.go`) — loads the feature's `CanonicalHead` via `changecontract.LoadHead` and prepends `changecontract.RenderHeadBlock(head)` before the existing `featurecatalog.HistorySlot` body. Verified with `TestFeatureHistorySourcePrependsCanonicalHead`/`TestFeatureHistorySourceNoHeadFallsBackToPriorBehavior`.
- [x] The Head block includes `behavior_statement`, `intent_signature` (short) + `status`, and the rejected-decisions ("do NOT re-attempt") list. (`changecontract.RenderHeadBlock`, `pack_test.go`.)
- [ ] Positive churn (`A → B → C → A`) is **not** replayed in the packed prompt by default; raw history is lower priority and its drop is logged. **Not done**: `RenderHeadBlock` only ever *adds* the Head block on top of the existing `HistorySlot` history — it does not make the raw history block itself lower-priority/droppable under the CP-23/CP-10 §5 packer's token budget, and no drop is logged. That packer-level integration was not located/built this pass; the Head is additive, not yet a substitute that can bump the older history out under pressure.
- [x] `spec_less` Heads render a low-confidence annotation. (`RenderHeadBlock`'s `statusChip`, `TestRenderHeadBlockAnnotatesSpecLess`.)
- [x] `canonical/*.json` syncs to `context-engine/` via the CP-35 P-8 mechanism with a manifest entry; `contracts.ndjson` never syncs. `EngineStore.SharedFiles()` now globs `canonical/*.json` (dynamic, one file per feature) alongside the existing fixed shared-file list; `WriteManifest` already iterates `SharedFiles()` so the manifest entry follows automatically. Verified with `TestSharedFilesIncludesCanonicalHeads`/`TestSharedFilesNeverIncludesContracts`.
- [ ] Desktop "Canonical Head" panel shows behavior, signature-status chip, rejected decisions, and the active step's in/out-of-scope diff. **Partially done, scoped down**: added `GET /client/projects/{projectId}/features/{featureKey}/canonical-head` and `GET /client/workflow-runs/{runId}/steps/{stepId}/contract` read endpoints (`canonical_head_handlers.go`), and a manual-lookup "Canonical Head" collapsible section inside `ProjectsSettings.tsx` (`apps/desktop-flowpilot`) showing behavior, status/spec-less annotation, the rejected/reverted decisions list, and a step's declared Contract (intent + declared paths). **Correction (2026-07-13)**: this was initially built in `apps/admin-web` before the owner clarified that app was dropped long ago and only `apps/desktop-flowpilot` is the active UI — the admin-web changes were fully reverted and the panel rebuilt in `ProjectsSettings.tsx` instead (a new "Canonical Head" collapsible section alongside the existing Overview/Bindings/Teams/MCP/Runs/Artifacts/Chat Sync sections, using the same `runnerFetch`/`readRunnerError` local helpers already defined in that file). This is a manual feature-key/run-id/step-id lookup form, **not** a per-project feature browser (no "list all features for this project" endpoint exists) and **not** a true in/out-of-scope diff against actually-touched files (that computation is `changecontract.ScopeDiff`, not yet exposed over HTTP — the panel only shows the step's *declared* paths).
- [ ] Manual: confirmed the packed prompt leads with the Head for both Claude and Codex turns. **Not done this pass** — no live E2E was run for Task-188 (unlike CP-41's manual verification earlier this project); only unit-level verification (`context_source_canonical_head_test.go`) confirms the section body ordering.
- [x] `go test ./internal/changecontract/... ./internal/flowgate/... ./internal/contextsync/... ./internal/runner/...` (targeted `TestFeatureHistorySource*`/`TestHandleGetCanonicalHead*`/`TestHandleGetStepContract*`) passes. `apps/desktop-flowpilot`: `npx tsc --noEmit` clean (no errors) after the `ProjectsSettings.tsx` panel addition; no component-test framework exists for this file in the current codebase (only a `test:phase1` node-based suite and one unrelated `.test.ts` helper file), so typecheck is the verification available and used.

## 7. Out of Scope

- Head/signature/drift computation (Task-186) and decision folding/retire (Task-187).
- Contract capture and scope rules (Task-184/185).
- Per-code-unit Heads (`SD-21 Q-4`).

## 8. Completion Notes

- result: **done (code, 2026-08-11).** T-1/T-3/T-4 done earlier; T-2 budget-drop on render path ([CA-435](../../change-audit/CA-435-cp43-p5-pack-budget-and-admin-scope-diff.md)); T-5 ScopeDiff API + feature list + panel scope view. Head via `canonical.head` source (Task-244), not prepend in `feature.history`.
- blocking for done: none (code). Manual B19/B20 tracked in CP-43-Test-Steps only.
- follow-ups: full CP-10 token packer (v1 uses char budget); live Claude/Codex prompt verify when operator runs B19/B20.
- upstream docs updated: none
