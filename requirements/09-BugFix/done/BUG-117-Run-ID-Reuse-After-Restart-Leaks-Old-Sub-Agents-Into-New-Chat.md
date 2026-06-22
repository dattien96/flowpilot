# BUG-117: Run ID Reuse After Restart Leaks Old Sub-Agents Into New Chat

## Metadata

- Document ID: `BUG-117`
- Title: `Run ID Reuse After Restart Leaks Old Sub-Agents Into New Chat`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [BUG-116: Child Agent Transcript And Panel Show Duplicate Entries](./BUG-116-Child-Agent-Transcript-And-Panel-Show-Duplicate-Entries.md), [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Replaces: `None`
- Tags: `multi-agent, agents-panel, run-id, restart, persistence, runner`

## AI Quick View

### Summary

- A brand-new chat showed sub-agents in the Agents panel before any `spawn_agent` was even approved — old reviewers from a previous session appeared under the fresh main run.
- Root cause: the runner mints ids from an in-memory `atomic.Int64` (`idCounter`) that resets to 0 on every restart. So a new chat reuses ids (`run-1`, `run-2`, …) that already belong to persisted runs from before the restart. The fresh `run-1` then collided with a previous `run-1`, and `listAgentRunSummaries` / the agent graph returned the old run's persisted child agents as if they belonged to the new session.
- Fix: at startup, seed `idCounter` above the highest numeric suffix among persisted run ids (and their parent ids), so freshly minted ids never collide with runs from before a restart.

### Current Ask

- A new chat must only show sub-agents it actually spawned — never inherit a previous run's children.

### Key Decisions

- `V-1` `seedIDCounter` scans `ListAllProviderSessions` at construction and advances `idCounter` to the max suffix found across `RunID` and `ParentRunID`. Best-effort: missing store / read error leaves the counter at 0.
- `V-2` Seeding from run ids is sufficient: run ids are the values used as cross-restart keys (parent references, history lookups). Event/bus id reuse across runs is harmless because those are scoped per run on the client.
- `V-3` The production `localFileSessionStore` loads persisted sessions from disk in its constructor (`loadFromDisk`) before the service is built, so the seed sees the full set.

### Constraints

- No id-format change (ids stay `prefix-N`), so tests and existing references are unaffected except that post-restart ids start higher.
- The desktop already clears `agentRuns` on new-run / history-open; the ghost children came from the server returning a colliding run's children, which this fixes at the source.

### Open Questions

- None. (Note: `TestProjectRunHistoryFiltersRunsByProject` is a pre-existing flaky test — an `updatedAt` ordering race, unrelated to this fix; flagged separately.)

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` — `idCounter`, `nextID`, new `seedIDCounter` / `numericIDSuffix`, constructor call
- `apps/local-runner/internal/runner/local_file_session_store.go` — `loadFromDisk`, `ListAllProviderSessions`

## 1. Issue Summary

After a runner restart, opening a new chat showed sub-agents (e.g. several "reviewer" rows) in the Agents panel that were never spawned in that session. They were children of a previous run whose id the new run reused.

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- task: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- related bugfix: [BUG-116](./BUG-116-Child-Agent-Transcript-And-Panel-Show-Duplicate-Entries.md)

## 3. Environment and Reproduction

- environment: Desktop + local runner with persisted chat history (localFileSessionStore), after at least one runner restart.
- reproduction steps:
  1. In a prior session, spawn one or more sub-agents under a run (e.g. `run-1` with child reviewers).
  2. Restart the runner.
  3. Start a new chat (which is minted as `run-1` again) — before approving any spawn, the Agents panel shows the previous run's child agents.
- frequency: Whenever a new run's reused id matches a persisted parent run that had children.

## 4. Expected vs Actual

- expected: ids are unique across restarts; a new chat has no sub-agents until it spawns them.
- actual: ids reset to 0 on restart and collided with persisted runs, so the new chat inherited the old run's children.

## 5. Impact

- users affected: multi-agent users after any runner restart.
- severity: High — the Agents panel showed unrelated/stale sub-agents, and run-id collisions risk cross-run state confusion.

## 6. Root Cause

- confirmed cause: `nextID` uses `s.idCounter.Add(1)` where `idCounter` is a zero-initialized `atomic.Int64` never seeded from persisted state. On restart it begins at 0, so `run-1`, `run-2`, … are reused. The persisted-session lookups that build the Agents panel (`listAgentRunSummaries`, `graphSnapshot`) key on `parentRunID`, so a reused parent id surfaced the prior run's children.
- evidence: runner log shows a new chat as `run-1` while persisted children (`run-14/15/16/17`) from earlier carried a `ParentRunID` matching a reused id; the panel listed them with no spawn performed.

## 7. Fix Strategy

- `F-1` Add `seedIDCounter` (called from `newInteractiveService`) that reads `ListAllProviderSessions` and advances `idCounter` to the max numeric suffix across persisted `RunID`/`ParentRunID` via a CAS loop.
- `F-2` Add `numericIDSuffix` helper to parse the trailing integer of an id.

## 8. Validation

- `V-1` `go build` / `go vet` clean.
- `V-2` `TestSeedIDCounterAvoidsRunIDReuseAfterRestart` — seeds a store with `run-9` and child `run-17`, constructs the service, asserts the next minted run id suffix is > 17.
- `V-3` `TestNumericIDSuffix` covers parsing (`run-14`→14, `main-run`→0, `run-`→0, `bus-007`→7, ""→0).
- `V-4` Manual: after rebuild + runner restart, a new chat starts with an empty Agents panel.
- `V-5` Pre-existing unrelated flaky test `TestProjectRunHistoryFiltersRunsByProject` (updatedAt ordering race) is not caused by this change — it flakes with and without it; flagged for separate hardening.

## 9. Regression Guard

- tests: the two new tests guard the seed and the parser.
- audit checks: a new chat listing sub-agents with zero spawns would indicate id reuse regressed.

## 10. Follow-Up Document Updates

- upstream docs that must change: None.
- follow-up: stabilize `TestProjectRunHistoryFiltersRunsByProject` (make history ordering deterministic under same-timestamp creation).
