# CA-1096: vibe audit auto-finalize settled done while the sprint was incomplete

Date: 2026-10-01
Refs: live production run-3362 (vibe-tasks, PrivateVault) — the run reported
`flow_run_complete_done` via `flow_audit_vibe_missing_key_auto` while
Task-015's contract suite was still red (9 failing tests, `TODO("not
implemented")` bodies in `MainActivity.kt`/`PrivaVaultApp.kt`) and the loop
carried `openIssues=3`.

## Root cause

`runAuditNode`'s `blocked_missing_feature_key` vibe auto-finalize (CA-814:
missing feature key is not operator-actionable, auto-resolve) never checked
sprint completeness. Open gate issues or a coder leg that never reached DONE
mean the work is mid-remediation — auto-finalizing there is a false green.

## Fix (`internal/runner/flow_validate_audit_dispatch.go`)

- New `vibeSprintEvidenceComplete`: `loopState.OpenIssues > 0` → incomplete;
  every declared `agent.code` node in the active topology must be DONE in the
  step table (unreadable step state fails closed). A topology with no coder
  leg (docs-only audit) has nothing to check — CA-814 behavior preserved.
- The auto-finalize branch now requires the check; otherwise the run falls
  through to the shared not-ready escalate with a sprint-specific summary
  ("sprint evidence incomplete …"), parking `WAITING_USER_APPROVAL` on the
  audit node — fail-closed, never a silent pass.

## Verification

- Red→green: `ca1096_audit_incomplete_finalize_test.go` —
  openIssues>0 → escalate (no done settle); declared coder leg not DONE →
  escalate; clean sprint still auto-finalizes.
- Pinned CA-814/CA-1074 audit tests still pass unchanged.
