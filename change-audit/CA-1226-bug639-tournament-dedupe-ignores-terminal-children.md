# CA-1226 — runner: BUG-639 tournament dedupe ignores terminal children

- **Area**: apps/local-runner/internal/runner (tournament_escalation.go)
- **Evidence**: live run-306526, 2026-10-06 — every
  `tournament_escalation` request after the first returned
  `deduped`/`already_running` because the dedupe scan matched the
  historical `run-306526-tournament` child purely by label. That child was
  terminal (stopped/cancelled) but its residue kept suppressing every
  fresh rescue escalation; the flow could never escalate to a real
  tournament again.
- **Bug**: both dedupe sites — the outer pre-check in
  `maybeEscalateCapToTournament` and the authoritative in-lock insertion
  check — iterated `s.runs` for `label == "tournament_escalation"` with
  no terminal/closed filter. A dead child satisfied dedupe identically to
  a live one.
- **Fix**: new helper `liveTournamentChildLocked(parentRunID)` returns the
  live (non-terminal run status, non-closed leg state) tournament child id
  or "". Both scan sites now consult it — concurrent dedupe is unchanged
  (still atomic under the same lock), while terminal/closed residue no
  longer blocks a fresh escalation.
- **Provider parity**: provider-agnostic — dedupe bookkeeping only.
- **Tests (additive, reproduce-first)**:
  `bug639_tournament_dedupe_terminal_test.go` —
  `TestTournamentDedupeIgnoresTerminalChild`: a parent with a *terminal*
  tournament child must dispatch a fresh escalation (red before fix —
  "terminal tournament child must not satisfy the dedupe"); a *live*
  tournament child must still dedupe (concurrent-rescue guard intact).
- **Verification**: focused tournament batch
  (`Tournament|tournament`) green; `TestMemberAction*` and cohort suites
  from CA-1224/CA-1225 still green.
- **Worktree**: n/a (runner-only change).
