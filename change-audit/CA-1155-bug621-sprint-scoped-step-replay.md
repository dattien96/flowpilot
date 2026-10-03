# CA-1155 — BUG-621: step-transition replay scoped to the current vibe sprint

Live trigger: run-150388 (the same cross-sprint contamination family as
BUG-616/620/622). The per-run step-transition sidecar
(`{run}-step-transitions.ndjson`) is keyed by node id only. vibe-sprint
remounts the same flow node ids (`context/tdd/coder/validate/reviewer/
synthesis/audit`) for every task, so after sprint 1 ended and sprint 2
(Task-032) mounted, restart reconstruction replayed sprint-1's terminal
lines onto the sprint-2 board:

- `coder|validate|reviewer|synthesis|audit` all replayed DONE before
  sprint-2's legs ever ran;
- a cancelled sprint-1-ish `tdd` leg's FAILED overlaid the completed
  sprint-2 tdd leg (terminal-monotonic — could never be overwritten);
- every board-reading path derived wrong state: `pendingVibeResumeFromNode`
  armed `Resume from context?` / `Resume from reviewer?` cards instead of
  `Resume from tdd?`, and `maybeResumeVibeCoderAfterTdd` refused to spawn
  because the poisoned board read coder as already DONE/RUNNING.

Root cause: `applyStepTransitionReplay` merged **all** logged lines with
node-id last-wins + terminal monotonicity. Session evidence had already
been sprint-filtered (BUG-616), but the transition-log merge was not —
the two evidence sources disagreed by design.

## Fix

- `stepTransitionLine` gains `VibeSprintIndex` (omitempty); the single
  append site `appendStepTransitionLog` stamps `rs.vibeSprintIndex` under
  the same lock discipline it already uses for `flowEngineDriven`.
- Replay moves to `applyStepTransitionReplaySprint(rows, lines, keep,
  currentSprint)`: when `currentSprint > 1`, lines stamped for a different
  sprint — and unstamped legacy lines, which cannot be attributed to a
  sprint — are skipped. The sprint-filtered session-evidence walk
  (BUG-616) rebuilds those rows correctly instead.
- `applyStepTransitionReplay` keeps its 3-arg signature delegating with
  `currentSprint = 0`: non-vibe runs, first-sprint runs, and the existing
  `v9_matrix_test` expectations replay every line exactly as before
  (byte-compatible legacy behavior).

Trade-off noted: dropping unattributable lines under sprint>1 is the
fail-closed choice — it also heals already-contaminated logs (like
run-150388's) on next resume, at the cost of losing posture-only stamps
(provider/model) that arrived unstamped.

## Tests

`bug621_sprint_scoped_replay_test.go` (additive):
- `TestBUG621_TransitionLineStampedWithSprintIndex` — append stamps 2.
- `TestBUG621_ReplayDropsForeignSprintTransitions` — sprint-2 replay drops
  sprint-1 DONE + unstamped DONE, keeps sprint-2 CANCELED.
- `TestBUG621_ReplayLegacySprintKeepsAllLines` — sprint 0/1 replay all
  lines unchanged.
