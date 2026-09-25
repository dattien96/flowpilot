---
id: BUG-449
title: Failed terminal append burns sequence and leaves phantom in-memory audit
status: done
version: 1
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-406, CA-922b]
---

## AI Quick View
- **What**: Terminal commit I/O failure leaves an audit entry and advanced seq in RAM with no matching durable record.
- **Why**: `appendAuditLocked` was moved before `commitLine` but does not roll back on error.
- **Key constraint**: Dispatch seq and audit log must remain contiguous and crash-consistent under failed writes, not only successful commits.

## 1. Metadata
- Document ID: `BUG-449`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `agent-flow-engine`
- Parent Documents: [BUG-406](../done/BUG-406-Dispatch-Seq-Collision-Terminal-Commit.md), [CA-922b](../../../change-audit/CA-922-dispatch-durability-fixes.md)

## 2. Symptom and Impact
CA-922b moved `appendAuditLocked` before `commitLine` to allocate a unique seq (`internal/runner/dispatch_store_memory.go:633-640`). `appendAuditLocked` increments `s.seq` and appends into `s.audits` immediately (`:112-117`). On failed `commitLine`, `commitTerminal` returns without restoring seq/audits (`:639-640`), while the dispatch record stays nonterminal. A subsequent successful retry writes a later sequence, leaving a gap; in-memory audit reports a terminal commit that never became durable. Severity: **medium/high** for audit ordering and replay guarantees. BUG-406 test validates the success path only (`bugg_cluster_dispatch_test.go:57`).

## 3. Reproduction / Evidence
Inject one `afterCommit` rejection of a terminal record, inspect `s.seq`/`s.audits`/durable lines, then retry and inspect contiguity. Code path is deterministic; **no new red fault-injection test run during review**.

## 4. Acceptance and Verification
Reserve seq without emitting audit until durable write succeeds, or rollback under error; add additive fault and restart tests, verify monotonic contiguous log and accurate in-memory audit after retry. Do not weaken successful-path test.

## 5. Resolution (2026-09-23, CA-932b)

- `commitTerminal` rolls back `s.seq`/audit state when `commitLine` fails —
  failed commits no longer burn sequence numbers; the durable log stays
  contiguous.
- Test: `bug447_449_durability_test.go` — forced commit failure then success
  yields contiguous seq values; red before fix.
