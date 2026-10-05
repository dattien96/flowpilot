# CA-1215 — Armed hub reinvoke starves deficient-member re-drive (live run-262417)

## Evidence
run-262417 (PrivateVault CP-04), 20:02→20:08 — a deterministic
park/continue cycle that could never converge:

1. `hub_notify_reinvoke_blocked` (20:02:54, 20:02:55) re-armed
   `rs.pendingHubReinvokePrompt` while the loop sat blocked on the
   review-verdict escalate
   (`synthesis done blocked — reviewer verdict not approved: spec_align=blocked`).
2. Every Continue drained the armed prompt FIRST and early-returned into
   `maybeAutoReinvokeHubWithPrompt` — a generic hub re-prompt.
3. The hub re-submitted `done` against the identical stale verdict →
   gate rejected → re-park → the reinvoke re-armed → next Continue
   repeated the cycle (20:04:47, 20:07:42, 20:08:01+).
4. The CA-1098 `resumeVerdictDeficientMembers` branch further down
   `resumeFlowWithFeedback` was unreachable: the armed-prompt
   short-circuit sits ~230 lines above it. The deficient `spec_align`
   leg stayed `waiting_user_approval` ~11 minutes until a continue
   happened to route elsewhere.

Earlier continues (19:49:07, 19:51:48, 19:54:18) DID re-drive the member
because no `hub_notify` had armed the prompt yet.

## Fix
In `resumeFlowWithFeedback`, after the unblock snap is emitted and before
the `pendingPrompt` short-circuit: when the park's gate reason belongs to
the hub-done verdict-gate family (`isReviewVerdictGateReason` or names a
deficient member verdict), attempt `resumeVerdictDeficientMembers` first.
The member's settle rejoins the cohort and re-invokes the hub on its own
edges, so the already-drained armed prompt is redundant. When no live
deficient member maps (deleted leg, no cohort), the code falls through
to the armed prompt exactly as before — BUG-284's same-node retry is
preserved.

## Tests
- `ca1215_armed_hub_reinvoke_starves_member_test.go`
  `TestCA1215_ArmedHubReinvokeDoesNotStarveDeficientMemberRedrive` —
  blocked/escalate loop + armed `pendingHubReinvokePrompt` + parked
  spec_align leg with `blocked` verdict: Continue re-drives the member
  leg (RED pre-fix: leg stays waiting_user_approval while only the hub
  re-prompts).
