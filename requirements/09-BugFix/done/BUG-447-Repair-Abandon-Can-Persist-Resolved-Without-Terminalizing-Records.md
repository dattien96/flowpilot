---
id: BUG-447
title: Repair abandon can persist resolved state without terminalizing all dispatches
status: done
version: 1
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-408, CA-922]
---

## AI Quick View
- **What**: Failed/crashed repair-abandon can be recorded as resolved while its dispatch remains non-terminal.
- **Why**: Repair resolution and each record terminalization are persisted as separate writes.
- **Key constraint**: Partial failure must remain safely retryable or explicitly uncertain; never report abandoned while records remain live.

## 1. Metadata
- Document ID: `BUG-447`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `agent-flow-engine`
- Parent Documents: [BUG-408](../done/BUG-408-CancelRequired-Unresolvable-Repair-Reopen.md), [CA-922](../../../change-audit/CA-922-dispatch-durability-fixes.md)

## 2. Symptom and Impact
`CommitRepairResolution` sets `r.State="resolved"`, advances `RepairRevision` and writes a `repair` line **before** looping over related nonterminal dispatch records (`internal/runner/dispatch_store_memory.go:1304-1326`). During `RepairResolvedAbandon`, each record is mutated in RAM and separately persisted as a `record` line (`:1327-1352`). Failure/crash after the resolved-repair write, or on any later record write, leaves durable `repair:resolved` while one or more dispatch records remain send_started/cancel_required. In RAM the failed record has already been marked terminal and its intent cleared despite `commitLine` failure. Resolution replay sees a resolved repair (`BeginRepairResolution` replay branch at `:1254-1264`) and does not finish terminalizing outstanding records.

This violates CA-922/BUG-408's repair guarantee and the three-outcome durable contract. Severity: **high**; whether boot scanner subsequently opens a new repair requires dedicated crash test, not presumed here.

## 3. Reproduction / Evidence
Use the local store's `afterCommit`/`persistLine` failure seam (`dispatch_store_memory.go:240-245`, `dispatch_store_local.go:61,117`) to fail the first `record` append after a successful `repair` append; compare RAM/disk state after restart. This is **code-path evidence**, not a completed fault-injection run in this review. Existing BUG-408 tests cover successful abandon, not write failure after the repair row.

## 4. Acceptance and Verification
Add additive red fault-injection and kill/restart E2E cases for 1 and N affected dispatches; ensure atomic resolution or a recoverable completion intent that deterministically finishes terminalization. Confirm error/replay semantics and provider-agnostic behavior; do not change old assertions to green.

## 5. Resolution (2026-09-23, CA-932)

- `CommitRepairResolution` terminalizes eligible live dispatch records first
  (each persisted), then writes repair `resolved` — a mid-path failure leaves
  the repair open with all records reachable on retry.
- Test: `bug447_449_durability_test.go` — abandon cannot resolve while live
  records remain; red before fix. `go test -count=1 -run Bug447` green.
