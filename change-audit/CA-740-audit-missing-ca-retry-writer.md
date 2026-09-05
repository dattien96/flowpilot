# CA-740 — audit missing-CA stamps audit; Retry re-enters writer to write the CA

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: bugfix
summary: an audit tier-3 missing change-audit-note escalate now parks the audit node (stamped as the escalated node) instead of plan_synthesis, and Retry on that park re-enters the nearest upstream agent.code writer with an explicit write-the-missing-CA prompt; the TUI Retry chip on a missing-CA park reads "continue: re-run the writer to write the missing change-audit note" instead of "old scope"
# --->8---

## Problem

- Live run-202550 (`/flow task-harness`, audit node): code changed but no change-audit note → `Audit gate (aggregate): Flow gate: code changed but no change-audit note found` → blocked/escalate with only `[Retry]`/`[Stop]`.
- Two defects stacked:
  1. The audit tier-3 escalate never stamped `lastEscalatedInlineNodeID`, so `applyFlowControl(escalate)` settled `plan_synthesis` WAITING via `setFlowStepAwaitingUser`'s first-hub fallback (screenshot: `plan_synthesis WAITING_USER_APPROVAL`, `audit RUNNING`). Retry then reinvoked the plan hub — re-running the plan instead of writing the CA — and the audit never re-ran.
  2. The `[Retry] - run again with old scope` copy (Task-309 drift wording) says nothing about the missing CA, so Continue looked impossible.
- (`Turn failed: interrupted by user` is the operator interrupting after the card, not a separate bug.)

## Changes

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` (`runAuditNode` tier-3 block): `stampLastEscalatedInlineNode(parentRunID, node.ID)` + settle the audit node WAITING directly (freeze-escalate shape, CA-623 branch in `applyFlowControl` re-settles idempotently). The plan hub is left alone — exactly one WAITING node.
- `apps/local-runner/internal/runner/interactive_service.go` (`resumeFlowWithFeedback` + helpers): on a missing-CA park (`isMissingChangeAuditNoteReason`) whose escalated node is `artifact.audit_draft`, Retry resolves the nearest upstream `agent.code` writer (`upstreamCodeWriterForNode`, forward-edges-only BFS: audit ← synthesis ← reviewer ← validate ← implement) and reinvokes its child with a "write the missing change-audit note (change-audit/CA-*.md), then continue" prompt. No writer/child → falls through to the existing audit re-dispatch (never strands the resume).
- `apps/local-runner/internal/tui/app/step_runtime.go`: missing-CA parks render `[Retry] - continue: re-run the writer to write the missing change-audit note`. Every other park keeps the exact established copy (pinned by `TestBlockedBar_RetryStopAlways_AllowOnlyOnDrift`); `[Allow]` stays drift-only; the banner chip list is unchanged.

## Tests added (new files only)

- `run202550_audit_missing_ca_retry_test.go`:
  - `TestRun202550AuditMissingCAStampsAuditNotHub` (Claude/Codex/Grok): real `runAuditNode` tier-3 r-ca → audit WAITING + stamped, plan_synthesis not WAITING, loop blocked/escalate.
  - `TestRun202550MissingCARetryReentersWriter` (matrix): resume → implement child `activationSeq==1` + turn prompt instructs the CA note; plan_synthesis not reinvoked.
  - `TestRun202550NonMissingCAAuditParkKeepsAuditRedispatch` (near-miss): other audit parks keep existing behavior, no writer-CA turn.
  - `TestRun202550UpstreamCodeWriterResolution` / `TestRun202550MissingCAReasonMatcher`: helper units incl. writer-less/unknown empties.
- `run202550_missing_ca_retry_copy_test.go` (tui/app): missing-CA chip copy per provider, generic escalate keeps old copy, matcher units.

## Verification

- New tests PASS; related old suites PASS untouched (BUG-327 incl. writer-reinvoke matrix, run147126, audit/validate dispatch, TUI blocked/drift/chip/banner suites): R1.
- Pre-existing environmental failures (stash-confirmed on baseline, unrelated): `TestResumeRunEchoesChatIdentity` (needs a real `~\.codex` session on this machine), `run144900` skill-drift (git-index env), intermittent Windows TempDir cleanup race.
- Provider-agnostic (R2): engine path + TUI copy take no providerKey; engine repro/routing matrix all three.
- Will not undo: CA-623 (escalated-node settle), BUG-289 A5 (hub-less re-entry + fallback preserved), CA-427 (Allow stays drift-only), Task-309 copy for all other parks.
- `go vet ./internal/runner/ ./internal/tui/app/` clean; runner binary builds.