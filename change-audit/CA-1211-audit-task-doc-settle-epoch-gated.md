# CA-1211 — Audit task-doc settle is sprint-epoch gated (live run-262417)

## Evidence
run-262417 (PrivateVault CP-04): `vibe_task_doc_settled` moved
`Task-042` todo→done at 20:16:49 — while sprint-042 had just mounted and
its tdd leg was still mid-turn. done/ progress counted 2/5 while real
delivery was 1/5.

## Root cause
`runAuditNode` resolved the sprint's task doc from the LIVE
`vibeSprintIndex`, and the settle (fs rename + plan re-point) ran in a
separate s.mu hold from the index read. An audit body landing across the
boundary take settles `plan[newIndex-1]` — the NEXT sprint's doc. Same
TOCTOU class as CA-1209 (which closed the stale-stamp half — the
synthesis=DONE that auto-fired this audit — but not the settle path
itself: an already-dispatched audit body can still land post-take).

## Fix
`settleVibeSprintTaskDocForEpoch`: the sprint index is captured at
audit-body start; the epoch check, the fs rename, and the plan re-point
run under ONE s.mu hold — a take cannot interleave between verify and
move. A mismatched epoch logs `vibe_task_doc_settle_stale_epoch` and
refuses (fail-closed: no rename, no repoint).

The terminal flow-side effects of a stale audit body (applyFlowControl
done sealing a new sprint) are covered by CA-1209's atomic settle —
the audit's own activation can no longer be produced by stale evidence —
plus `auditCtxCancelled` guards at each stage.

## Tests
- `ca1211_audit_doc_settle_epoch_test.go`
  `TestCA1211_StaleEpochRefusesToMoveNextSprintDoc` — captured index 1
  vs current 2: no rename, no repoint (RED verified by disabling the
  epoch check — the doc moved).
  `TestCA1211_CurrentEpochSettlesOwnDoc` — same-epoch settle still moves
  todo→done and re-points the plan entry.
