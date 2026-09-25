# BUG-392: HTTP `/flow-control` bypasses `validateReviewACCoverage` — per-AC enforcement is bridge-only

## Metadata

- Document ID: `BUG-392`
- Title: `HTTP flow-control face skips validateReviewACCoverage; delegate-child guard runs before AC validation`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-62-Test-Steps](../../07-Coding-Plan/done/CP-62-Test-Steps.md), [CP-48-Test-Steps](../../07-Coding-Plan/done/CP-48-Test-Steps.md)
- Feature Keys: `zcode-parity`, `agent-flow-engine`

## AI Quick View

### Summary

- `validateReviewACCoverage` (`review_ac_coverage.go:40`) is invoked only from `bridgeSubmit` (`interactive_service.go:6313`) — the provider MCP path. The HTTP endpoint `POST /client/workflow-runs/{runId}/flow-control` (`interactive_handlers.go:1627`) maps the body to `FlowControlInput` and calls `applyFlowControl` directly (`interactive_handlers.go:1715`) — AC validation never runs on the HTTP face.
- Live probes (run-2737/run-4160): an `approved` submit with a **partial** `verdicts` array over HTTP was rejected only by the machine-verdict presence check (or silently accepted with `loopState.done`) — never by per-AC coverage.
- Second symptom (folded in): inside `bridgeSubmit` itself, the non-cohort delegate-child guard (`interactive_service.go:6303`) fires **before** `validateReviewACCoverage` (`:6313`) → AC rejection is unreachable for delegate children — deterministic unit failure `TestReviewACCoverage_BridgeSubmit_RejectsBeforeProcessing` (CP48-1).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** Operator/UI submits of review outcomes via `POST /flow-control` skip the per-AC completeness check entirely; and on the bridge path, delegate children hit the delegate guard before coverage so the AC error can never surface to them.
- **Expected:** `validateReviewACCoverage` applies uniformly on both submission faces; a delegate child submitting a verdict payload gets the AC-coverage error, not a generic delegate rejection.
- **Actual:** HTTP face → `applyFlowControl` directly (no coverage). Bridge face → delegate rejection precedes coverage for non-cohort children.
- **Impact:** A board/UI (or any HTTP client) submitting `approved` with partial `verdicts` bypasses the completeness contract — missing-AC-row rejection exists only on the provider tool path. Mitigating factor: hub `approved` still requires a recorded machine verdict via `recordReviewCohortMemberVerdict` (bridge-only), so verdict rows don't register — but the coverage contract is not enforced uniformly. Live CP-62 L-62-3 could not exercise missing-AC rejection end-to-end for this reason.

## Reproduction

1. Run `task-harness` to a parked reviewer/synthesis checkpoint (run-2737, opencode).
2. `POST /client/workflow-runs/run-2737/flow-control {"status":"approved","verdicts":[<only AC-1 row>]}` → `422` but from `missing machine verdict from reviewer(s)`, not coverage.
3. `POST /client/workflow-runs/run-4160/flow-control` (pending reviewer child) same partial body → bare `AgentGraphSnapshot` with `loopState.status:"done"`, no validation error, no verdict recorded.
4. Automated: `go test ./internal/runner/ -run TestReviewACCoverage -v` → `BridgeSubmit_RejectsBeforeProcessing` FAILS (delegate guard fires first).
- runIds: `run-2737`, `run-4160`, `run-2849` (CP-62); CP48-1 unit evidence (run-11 reviewer children run-815/run-3142 context).

## Root cause

- `apps/local-runner/internal/runner/interactive_handlers.go:1627` — `handleSubmitFlowControl`; `:1715` calls `s.applyFlowControl(runID, in)` directly, no `validateReviewACCoverage`.
- `apps/local-runner/internal/runner/interactive_service.go:6313` — sole `validateReviewACCoverage` call site, inside `bridgeSubmit` (provider tool face).
- `apps/local-runner/internal/runner/interactive_service.go:6303` — non-cohort delegate-child rejection precedes the coverage check at :6313 (ordering bug; also the cause of the automated failure noted in CP-62's suite).

## Evidence

- `~/fp-beds/lt-evidence/cp62/RESULT.md` (L-62-3 notes; Bugs BUG-LIVE-3)
- `~/fp-beds/lt-evidence/cp62/BUG-LIVE-3.md` — probe detail + ordering analysis
- `~/fp-beds/lt-evidence/cp62/l62-3-run2737-agentgraph.json` — post-probe state
- `~/fp-beds/lt-evidence/cp62/autotest-cp62-followup.log` — `TestReviewACCoverage_BridgeSubmit_RejectsBeforeProcessing` FAIL
- `~/fp-beds/lt-evidence/cp48/RESULT.md` (BUG-LIVE-CP48-1 — ordering; `autotest-ac-coverage.txt` 8/9 PASS)

## Severity

- `high` — enforcement parity gap between the provider tool face and the HTTP/operator face on a CP-62 acceptance mechanism; AC coverage is unenforced where a human/UI submits.

## Completion Notes (implemented 2026-09-23, CA-920b)

- `turnBridge.SubmitFlowControl` now runs `validateReviewACCoverage` before routing; the delegate-child guard no longer pre-empts AC rejection for reviewer children.
- HTTP `handleSubmitFlowControl` maps review outcomes through `reviewOutcomeToFlowControl` and enforces the same coverage check — both faces now reject partial/missing `verdicts`.
- Unit+E2E: `TestBug392_HTTPFlowControlEnforcesACCoverage`, `TestReviewACCoverage_BridgeSubmit_RejectsBeforeProcessing` + 10 edge-case tests — all green.
