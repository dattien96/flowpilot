# CA-297 — Canonical-Head Packing And Admin Visibility (Task-188, CP-43 P-5)

## Scope

Implemented (partially — see caveat) [Task-188](../requirements/08-Task/todo/Task-188-Canonical-Head-Packing-And-Admin.md) (P-5 of [CP-43](../requirements/07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md), the final slice): make the packed prompt lead with the [Canonical Head](CA-295-canonical-head-and-intent-signature.md) before raw history, sync `canonical/*.json` via the existing Drive mechanism, and give operators a minimal panel in the desktop app (`apps/desktop-flowpilot`) — `apps/admin-web` is deprecated/dropped by the project owner and is not touched.

## Changes

- New `apps/local-runner/internal/changecontract/pack.go`: `RenderHeadBlock(h)` — the mandatory, lead-first prompt block: `## Canonical state of "<feature>"` with `behavior_statement`, a truncated `intent_signature` + status chip (annotated `(spec-less — low confidence)` when applicable), and a "Do NOT re-attempt" list built from rejected/reverted `Decisions`.
- `runner/context_sources_builtin.go`: `featureHistorySource.Fetch` (the CP-44 `feature.history` ContextSource, used by flow-mode coding turns) now loads the feature's Head via `changecontract.LoadHead` and prepends `RenderHeadBlock` before the existing `featurecatalog.HistorySlot` body. A missing/unreadable Head degrades to "no Head block" (never an error). Note: this also changes when the pre-existing "no change history found" warning fires — a feature with a Head but no raw history now gets Head content instead of the warning, which is intentional (there is something to show).
- `contextsync/local.go`: `EngineStore.SharedFiles()` now also globs `canonical/*.json` (an unbounded, one-file-per-feature set, unlike the other fixed shared paths) into the Drive-synced set; `contracts/contracts.ndjson` (Task-184, local-only) is untouched since it lives under a different directory this glob never reaches. `WriteManifest`/`SyncSharedFiles` needed no changes — both already iterate `SharedFiles()`.
- `runner/canonical_head_handlers.go` (new): two read endpoints — `GET /client/projects/{projectId}/features/{featureKey}/canonical-head?workingDirectory=...` (mirrors `handleGetEngineGateConfig`'s workingDirectory-query-param resolution) and `GET /client/workflow-runs/{runId}/steps/{stepId}/contract` (resolves the run's workspace from the live `interactiveRun`, then `changecontract.Store.Get`). Registered in `interactive_handlers.go`.
- `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`: a new "Canonical Head" collapsible section (alongside the existing Overview/Bindings/Teams/MCP/Runs/Artifacts/Chat Sync sections in this same per-project detail view) with two lookup forms — feature key → Head, run id + step id → Contract — rendering behavior statement, status + spec-less annotation, the rejected/reverted decisions list, and the step's declared paths. Uses `selectedProject.directoryPath` as `workingDirectory` and the file's existing local `runnerFetch`/`readRunnerError` helpers (no new API-client abstraction needed).
- **Correction**: this was initially built in `apps/admin-web` (new types, gateway methods, a new TanStack Router route, a `ProjectSectionNav` tab) before the project owner clarified that app was dropped long ago and is not used — those admin-web changes were fully reverted (`git checkout` on the 6 modified files, delete on the 2 new files) and the panel was rebuilt in `apps/desktop-flowpilot` instead, per above.

## Caveat — not fully done

Two gaps, left explicit:

1. **No packer-budget integration.** `RenderHeadBlock` only *adds* the Head block on top of the existing raw-history body — it does not make that raw history itself lower-priority/droppable under a token budget, and nothing logs a drop. The CP-23/CP-10 §5 packer this task's doc referenced for that behavior was not located/built this pass; positive churn (`A → B → C → A`) is still present in the packed prompt whenever `featurecatalog.HistorySlot` includes it, the Head block is additive truth on top, not a replacement.
2. **Desktop panel is a manual-lookup MVP, not a browser.** There is no "list features for this project" endpoint, so the panel is a feature-key text box, not a dropdown/list. It shows the step's *declared* paths (from the Contract), not a true in/out-of-scope diff — that computation (`changecontract.ScopeDiff`, Task-185) is not exposed over HTTP yet.

No live manual E2E was run confirming the packed prompt actually leads with the Head for a real Claude or Codex turn (unlike CP-41's live-verified scenarios earlier this project) — only unit-level verification of the section body ordering. Task-188 stays `in_progress`; CP-43's `P-5` DoD checkbox is left unchecked with these gaps spelled out.

## Verification

- `go build ./...` clean; `go vet` clean on `changecontract`, `flowgate`, `contextsync`, `runner`.
- `go test ./internal/changecontract/... ./internal/flowgate/... ./internal/contextsync/...` — 196 passed (5 new `pack_test.go` cases, 2 new `contextsync_test.go` cases for canonical-glob-inclusion/contracts-exclusion).
- `go test ./internal/runner/...` (targeted `-run "TestFeatureHistorySource|TestHandleGetCanonicalHead|TestHandleGetStepContract"`) — 6 passed.
- `apps/desktop-flowpilot`: `npx tsc --noEmit` — clean, no errors (including in the touched `ProjectsSettings.tsx`). No component-test framework exists for this file in the current codebase (only a `test:phase1` node-based suite and one unrelated `.test.ts` helper), so typecheck is the verification used, consistent with this file's existing coverage level.
- `apps/admin-web` was left untouched (all exploratory changes there were reverted) — confirmed clean via `git status --short apps/admin-web` showing no output.
- Full `go test ./internal/runner/... -count=1` regression across all of Task-186/187/188 together — 1392 passed, 16 failed (pre-existing baseline — Codex CLI resume, account-home, skills-merge, auth-workspace tests), 18 skipped. Zero new failures, closing out the CP-43 P-1..P-5 pass per the owner's "test only when all done" instruction.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-188
change_type: feature
summary: coding-turn prompts now lead with a feature's Canonical Head (behavior, signature status, rejected decisions) before raw history, canonical/*.json syncs via the existing Drive mechanism, and a minimal panel in the desktop app's Projects settings exposes Head/Contract lookups by feature key / run+step id — packer-budget demotion of raw history and a real feature browser/diff view remain unbuilt; an initial admin-web implementation was reverted since that app is deprecated
# --->8---
