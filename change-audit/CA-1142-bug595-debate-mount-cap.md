# CA-1142 — BUG-595: gate-triggered owner-debate mounts are unbounded

- **Bug:** BUG-595 (live run-100368). A gate divert mounted an owner debate,
  the debate resolved, the sprint resumed — then the next post-turn gate on
  the same gated child mounted another debate. Seven sequential debates on
  one child burned ~8h of wall time before the run ended `waiting_question`
  and was killed. There was no counter anywhere: `stashVibeFlowForDebate`
  only refused when a debate was *currently* mounted, so a debate that
  resolved legitimately reset the entire budget.
- **Fix:** `rs.vibeDebateMounts` counts every fresh park
  (`stashVibeFlowForDebate` increments on the successful-park path only —
  recorded-but-not-reparked second diverts do not consume budget).
  `startVibeOwnerDebate` checks the count before stashing: at
  `maxVibeDebateMountsPerSprint = 3` the divert escalates through
  `escalateVibeDebateMountCap` — the same `blocked`/`cap` loop surface and
  tournament-rescue seam (`maybeEscalateCapToTournament`) the owner-fail cap
  already uses, so the run parks for a human instead of looping debates.
- **Reset boundary:** `takeNextVibeSprintLocked` zeroes the counter when a
  sprint is actually taken — each task gets a fresh remediation budget, and
  the counter never resets mid-sprint on a resolve-and-remount.
- **Durability:** `VibeDebateMounts` rides `ProviderSessionState` +
  `ndjsonSessionRecord` (`vibe_debate_mounts`) through
  `sessionStateOf`/`sessionStateFromRecord`/`reconstructRunInternal` — a
  restart mid-sprint cannot launder the cap back to zero. Invalid
  pending-flow records clear it with the rest of the claim state.
- **Contract preserved:** a debate that is already mounted when the cap is
  reached is never killed — the check requires no live claim and no mounted
  debate graph, so an in-flight remediation always concludes; suppression
  (the gated child recorded on an existing claim) still does not consume
  budget. At cap the run gets a structured escalation, never a silent stop.
- **Tests:** `bug595_debate_mount_cap_test.go` — red before fix: mount count
  increments per fresh park and resets on `takeNextVibeSprintLocked`;
  cap-exceeded divert escalates (blocked/cap park or tournament rescue)
  rather than mounting a fourth debate.
- **Refs:** BUG-595. Live evidence: run-100368 flow-diag — seven sequential
  `vibe_debate_mounted` events on the same gated child across ~8h.
