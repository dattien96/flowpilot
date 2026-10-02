---
document_id: BUG-560
title: Audit escalates while an upstream cohort member is still in flight — the park kills it mid-turn
phase: bugfix
status: done
owner: flowpilot-team
reviewers: flowpilot-team
created: 2025-01-01
last_updated: 2025-01-01
parent_documents: CP-02
child_documents: ""
related_documents: BUG-559, CA-1096, CA-1074, run-60899
replaces: ""
tags: agent-flow-engine, audit, cohort, vibe-tasks, regression
---

## AI Quick View

### Summary

- Live run-60899: reviewer leg `run-67320` spawned into cohort
  `flow-auto-validate-round-1` at 10:31:52; 17s later the audit node ran,
  evaluated `blocked_missing_feature_key` while `openIssues=2` (stale),
  escalated, and `parkFlowForAwaitingUser` cancelled the reviewer's turn
  mid-flight (`cohort_member_failed ... "interrupted by user"`).
- The not-ready verdict was provisional — upstream evidence (the review
  verdict) was still in flight. Escalating on it discarded real work and
  drained the cohort barrier as cancelled, leaving nothing to rejoin on resume.

### Current Ask

- In `runAuditNode`, when `draft.Status != "ready"` and the run still has an
  **open cohort** (expected members not yet buffered), defer instead of
  escalating: reset the audit step to PENDING, keep the loop running, and let
  the cohort join re-drive the audit (join → synthesis hub → done edge →
  audit re-dispatch). No park, no cancel.
- Infra escalates (git observe failure, draft persist failure) are unchanged —
  only the verdict-driven not-ready path defers.
- `ready` → done is already guarded by `applyFlowControl`'s open-cohort
  soft-defer; this fix closes the asymmetric hole on the escalate side.

### Key Decisions

- Generic (not vibe-only): any not-ready audit while a barrier is open is a
  premature verdict for every topology; deferral is strictly safer than
  cancelling live member work.
- PENDING (not RUNNING) on defer: the node is awaiting re-dispatch, not
  executing; the re-drive happens through the normal done-edge path when the
  cohort resolves.

## 1. Issue Report

- run-60899 ndjson 10:32:09: `flow_audit_blocked_not_ready` →
  `flow_control_escalate` → `cohort_member_failed run-67320 "interrupted by
  user"` — the escalate park cancelled the reviewer mid-review and the cohort
  barrier drained on the released (cancelled) seat.

## 2. Reproduction

`bug560_audit_open_cohort_defer_test.go`: CA-1096 sprint fixture + a
pre-registered open cohort → `runAuditNode` must not block the loop nor stamp
WAITING; audit resets to PENDING; after the cohort drains, a re-run escalates
normally.

## 3. Diagnosis

- `runAuditNode`'s `draft.Status != "ready"` block escalates unconditionally.
  `applyFlowControl` soft-defers `done`/`continue` on open cohorts, but the
  escalate → `parkFlowForAwaitingUser` path has no symmetric check and cancels
  every in-flight child — including the cohort member whose result the audit
  was waiting on.

## 4. Fix Plan

- `flow_validate_audit_dispatch.go`: open-cohort defer inside the not-ready
  block before the escalate.

## 5. Implementation

- `runAuditNode`: inside the `draft.Status != "ready"` block, an open cohort
  barrier (`hasOpenCohort`) defers the not-ready verdict — the audit step is
  reset to PENDING and the loop keeps running; the cohort join re-dispatches
  the audit through the normal done-edge path. Infra escalates (git observe,
  draft persist) and the ready→done path are untouched.

## 6. Acceptance Criteria

- [x] Red → green regression test (`bug560_audit_open_cohort_defer_test.go`,
  3 cases: defer while open, escalate after resolve, no defer without cohort).
- [x] CA-1096/CA-1074/BUG-356 audit suites unchanged and green.
- [x] In-flight cohort members are never cancelled by a verdict-driven audit
  escalate while their barrier is open.

## 7. Human Confirmation

- pending
