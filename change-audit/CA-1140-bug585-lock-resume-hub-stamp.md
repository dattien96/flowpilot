# CA-1140 — BUG-585: lock/requirement resume re-stamps DONE hub node RUNNING

- **Bug:** BUG-585 (live run-139670). Confirming the CP Preview & Lock gate on
  `vibe-tasks` called `resumeFlowWithFeedback`, whose generic "un-park the
  escalated hub" stamp re-stamped `cp_validator` — already `DONE` — back to
  `RUNNING`. `vibe_lock` parks `cp_lock` (a `user.confirm` node), not the hub,
  and `resumeVibeLock` advances `cp_lock → task_plan_reader` without any hub
  turn. The RUNNING stamp had no leg behind it and never settled: the node
  stayed RUNNING forever with no card to answer — the "coder hang" phantom
  wedge in the step timeline.
- **Root cause:** `escalatedIsHub = lastEscalatedInlineNodeID == ""` treats
  "no remembered escalated node" as "the hub itself parked". That is true for
  escalate/cap parks (`setFlowStepAwaitingUser` stamps the hub WAITING first)
  but false for `vibe_lock` and `requirement` parks, which never stamp
  `lastEscalatedInlineNodeID` and never park the hub.
- **Fix:** the hub-RUNNING stamp now additionally requires the hub step to
  actually be `WAITING_USER_APPROVAL` (the escalate/cap park's own stamp), or
  a real hub reinvoke prompt to be owed (`pendingPrompt != ""`). A DONE or
  idle hub is no longer resurrected by a non-hub gate's resume.
- **Contract preserved:** the BUG-231/BUG-233 un-park still fires for real hub
  parks (guard test asserts a WAITING hub flips back to RUNNING on resume);
  run-147126 no-flap behavior for writer/audit parks (`esc != ""`) unchanged.
- **Tests:** `bug585_lock_resume_hub_stamp_test.go` — red before fix:
  DONE `cp_validator` stayed DONE after a `vibe_lock` resume; WAITING hub on
  an escalate resume still flips RUNNING.
- **Refs:** BUG-585. Follows BUG-588 (same class: RUNNING stamp without a leg
  behind it), fixed independently at the stamp site rather than the store.
