# CA-1187 — leg rotation on a live flow-loop owner orphans the sprint

## Evidence (live run-204891 → run-225672, CP-03 Task-039 sprint, 2026-10-05)

```
06:22     provider-compacted hub context → pendingQuestion
06:23:32  rotate_leg committed → run-204891 legSeq 0 CLOSED
          (legClosedReason=context_reset), run-225672 minted legSeq=1
          ChatMode="normal_chat", loopState EMPTY, no flow children
06:24→06:48  orphan leg ran the sprint's coding work as a plain chat run —
          no gate/signature-lock guards (complied by luck); sprint loop
          kept waiting on the dead hub leg → hub_stalled deadlock
          (downstream poison = BUG-1188)
```

## Root cause

`contextResetEligibleLocked` only checked `parentRunID=="" &&
chatID!="" && legState==active` — it never asked whether the run OWNS A
LIVE AGENT LOOP. `switchChatLeg` Phase-A guards covered a busy leg
(turn/approval/question pending) but not an idle-loop sprint hub waiting
between turns. A minted leg carries `FlowArm`/`FlowRefFallback` for a
PENDING latch only (CP-89 Task-451) — nothing transfers an in-flight
loop binding: nodes, gate state, cohort topology, and the loop record all
stay keyed on the closed source run.

## Fix

- `contextResetEligibleLocked` → `(s *InteractiveService)` method:
  permanent-shape half split into `contextResetShapeEligibleLocked`
  (root + chat + active leg), plus a live-loop refusal —
  `loopStateFor(rs.id).Status` not in `{"", "done", "stopped"}` is
  ineligible. Same terminal classification as
  `flowHubCompletionWithheldLocked`. Sealed loops stay eligible (the run
  is plain chat per BUG-302/308); pending latches keep riding switches.
- `switchChatLeg` Phase-A guard (same classification): refuses
  `flow_in_progress` 409 — covers every leg-minting caller (provider
  switch AND context rotate_leg), not only the reset path.
- `consumePendingContextReset`: shape-ineligible drops the intent (can
  never fire); a live-loop refusal DEFERS — the committed reset executes
  at the first admission after the flow seals, instead of silently
  dropping a user commitment.

## Regression

`internal/runner/bug1187_context_reset_flow_hub_test.go` — four
red→green shapes: live statuses (running/blocked/paused/waiting_review)
ineligible; no-loop/sealed stays eligible; `postSwitch` on a live-loop
owner gets 409 `flow_in_progress`; a committed pending reset defers
armed on a live loop and re-eligible after `done`. Switch suite
(Task-314), BUG-451/452/489/516/541, CP-89 Task-451 all green.
