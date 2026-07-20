# CA-375: Chat-mode Review Loop runs no longer get stuck "running" after review-outcome submission

## Summary

Fixed BUG-298: a chat-mode run using the built-in "Review Loop" orchestration got permanently stuck at `status="running"` after its hub called `submit_review_outcome` following an earlier completion checkpoint. Confirmed with real production data (`sessions.ndjson` + `dispatch.ndjson` for the actual affected run): the session-level `pendingFlowGateSettle` flag armed correctly (via `markPendingFlowGateSettleLocked`, gated on `flowEngineDriven`), but the dispatch ledger's `SettleOwed` — computed by `requiresGateSettlement(runMode, stepID)` — was persisted `false` for every turn of every chat-mode run, because that predicate only recognized `runMode` values `"flow"`, `"workflow"`, or `""`, never `"chat"`. With `SettleOwed=false`, the live settle-drive scheduler (`maybeScheduleSettleAfterTerminal`, Task-251) never re-ran the post-turn gate check that would flip `status` back to `Completed`, leaving the run stuck until a server restart's boot-time recovery (which reads `pendingFlowGateSettle` directly, bypassing `SettleOwed`) resolved it.

Root cause was an original design gap in Task-248 (`914dce7`, "durable dispatch record store", 2026-07-17) — introduced 12 hours after `b47d4c4` (BUG-288, 2026-07-16) had just taught `flowEngineDriven` to recognize chat-mode Review Loop runs, without Task-248's own parallel settle-tracking accounting for that same case.

Fix: `requiresGateSettlement` now also recognizes `runMode == "chat"`. Confirmed provider-agnostic — the function takes no `providerKey` and the fix applies identically to Claude, Codex, and Grok.

## Verification

- `go test ./internal/runner -run TestBug298 -count=1`: 6 passed (the fix itself, non-regression for flow/workflow/empty, and an end-to-end `newDispatchRecord` check parameterized across Claude/Codex/Grok).
- `go test ./internal/runner -run 'TestDispatch|TestSettle|TestGate|Test.*Settle|TestRun1264|TestRun2383|TestBug288|TestBug289|TestBug298' -count=2`: 294 passed, run twice — fully deterministic.
- No existing test called `requiresGateSettlement` directly, so nothing was weakened; additive-tests-only honored.
- `-race` not run: this machine lacks gcc/CGO.
- GitNexus impact analysis unavailable (`%1 is not a valid Win32 application`); localized tracing performed instead — enumerated both gates (`flowEngineDriven`, `requiresGateSettlement`) and every settle-scheduling call site, and cross-referenced the exact real `sessions.ndjson`/`dispatch.ndjson` records for the affected run/turn before changing the single shared predicate.

## Files

- `apps/local-runner/internal/runner/dispatch_record.go`: `requiresGateSettlement` recognizes `runMode == "chat"`.
- `apps/local-runner/internal/runner/bug298_chat_mode_gate_settlement_test.go`: additive regression + non-regression tests, parameterized across Claude/Codex/Grok.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-298
change_type: bugfix
summary: The dispatch ledger's settle-owed computation now recognizes chat-mode runs, so a Review Loop run's hub turn (e.g. submit_review_outcome) correctly re-triggers the post-turn gate check and the run no longer gets stuck at status=running until a server restart.
# --->8---
