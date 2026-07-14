# CA-298 — Canonical Head Lifecycle Actions: attach-spec rebaseline + retire wiring (Task-186/187, CP-43 P-3/P-4)

## Scope

Follow-up that closes the two "trigger" gaps explicitly left open in [CA-295](CA-295-canonical-head-and-intent-signature.md) (Task-186 attach-spec) and [CA-296](CA-296-superseding-decision-records-and-retire.md) (Task-187 retire): both `r-attach-spec` and `r-retire` were previously detect-only / dormant with no way for a human to actually resolve them. They are now fully round-trippable via explicit desktop actions.

## Changes

- `apps/local-runner/internal/runner/canonical_head_handlers.go` — two new POST endpoints:
  - `POST /client/projects/{projectId}/features/{featureKey}/canonical-head/rebaseline` — resolves the feature's governing DocRefs from the catalog and calls `changecontract.RebaselineWithSpec`, flipping a `spec_less` Head to `current` (the human-confirmation caller for Task-186's `r-attach-spec`). 400 when the feature has no governing docs registered; 404 when no Head exists.
  - `POST /client/projects/{projectId}/features/{featureKey}/canonical-head/retire` — body `{workingDirectory, action, targets[]}`; validates action ∈ {renamed, merged, deprecated}; rename/merge require ≥1 target; mints a target Head via `BuildHead` if a successor doesn't exist yet; calls `changecontract.RetireHead` and `SaveHead`s the retired Head + each updated target.
  - Both registered in `interactive_handlers.go`; a shared `toCanonicalHeadResponse` helper was extracted.
- `apps/local-runner/internal/runner/gate_hook.go` — new `detectRetirePending(cwd, knownFeatureKeys)`: sets `TurnResult.HeadRetirePending` (so `r-retire` fires as a warn nudge) when a **spec-backed, not-yet-retired** Canonical Head's key has vanished from `change-audit/FEATURE-KEYS.md`. `spec_less` Heads are exempt (their key may be an inferred, never-registered one) — keeps the nudge low-noise. Wired into the `runFlowGate` `TurnResult` literal.
- `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx` — in the existing "Canonical Head" section: a "Confirm spec & rebaseline" button shown when the looked-up Head is `spec_less`, and a retire form (action select + comma-separated targets input + "Retire feature" button). Both POST via the file's existing `runnerFetch`/`readRunnerError` helpers and refresh the displayed Head from the response.

## Design decision (resolves SD-21's Open Question)

Both transitions are **explicit user actions in the desktop app**, not the SD-16 flow-gate approval modal and not auto-detected from a `FEATURE-KEYS.md` diff. Rationale: these are deliberate, infrequent human judgements ("does this spec describe current behavior?" / "is this feature really being merged into X?"). A diff-based auto-detector is fragile (a rename looks like delete+add; a typo fix looks like a retire) and an inline gate modal would nag every turn. The gate rules `r-attach-spec` / `r-retire` remain as *nudges* that surface the pending state until the human acts; `r-retire`'s nudge is now backed by the conservative `detectRetirePending` heuristic.

## Verification

- `go build ./...` clean; `go vet ./internal/runner/` clean.
- `go test ./internal/changecontract/... ./internal/flowgate/...` — 193 passed.
- `go test ./internal/runner/ -run "TestHandleRebaseline|TestHandleRetire|TestDetectRetirePending|TestHandleGetCanonicalHead|TestHandleGetStepContract|TestFeatureHistorySource"` — 11 passed (2 rebaseline incl. no-docs 400, 2 retire incl. rename-requires-targets, 1 detectRetirePending, plus the prior 6).
- `apps/desktop-flowpilot`: `npx tsc --noEmit` clean.
- Full `go test ./internal/runner/... -count=1` regression — **1466 passed, 16 failed, 18 skipped**. The 16 failures are the identical pre-existing baseline (Codex CLI resume, account-home, skills-merge, google-drive-mcp provider count, auth-workspace) tracked all session — zero new failures from this change.

## Supersedes

- The "Caveat — not fully done" item #2 in CA-295 (attach-spec had no confirmation caller) — now resolved.
- The "Caveat — not fully done" item #2 in CA-296 (`r-retire` could not fire; no retire trigger) — now resolved (explicit action + conservative nudge). The FoldDecisions keyword-matching limitation in CA-296 item #1 stands unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-187
change_type: feature
summary: canonical-head attach-spec rebaseline and feature retire (rename/merge/deprecate) are now explicit human-initiated desktop actions backed by two POST endpoints, closing the previously-dormant r-attach-spec and r-retire rules; r-retire also fires as a conservative FEATURE-KEYS.md-based nudge via detectRetirePending
# --->8---
