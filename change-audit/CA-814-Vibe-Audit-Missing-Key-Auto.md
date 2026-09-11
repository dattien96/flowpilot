# CA-814 — vibe audit missing feature key auto-finalizes

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Vibe audit does not park on missing feature key; auto-finalizes so Retry cannot loop
# --->8---

## Why

Live vibe-sprint: audit escalated "feature key missing or unverified". That
is not operator-actionable (sandbox has no catalog key). Retry re-ran the
same check and re-parked.

CP-60: non-requirement gate fails auto-resolve in vibe.

## Change

- `isVibeWorkingMode`
- `runAuditNode`: vibe + `blocked_missing_feature_key` → applyFlowControl
  done + audit DONE. Non-vibe still escalates.

## Tests

ca814_vibe_audit_missing_key_test.go: vibe auto-done; non-vibe escalate;
Retry does not re-park.

## Providers

Agnostic Case 1.

## Will not undo

V9-01 rag-harness missing-key escalate. blocked_validation_failed still
parks.
