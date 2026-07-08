# BUG-250: Restarted Flow Hub Run Permanently Unresumable — Placeholder Session Never Bypassed

## Metadata

- Document ID: `BUG-250`
- Title: `Restarted Flow Hub Run Permanently Unresumable — Placeholder Session Never Bypassed`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-07`
- Last Updated: `2026-07-07`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Scenario 5), [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (hub-reinvoke / suppressed first turn)
- Child Documents: `none`
- Related Documents: [Task-067: Desktop Post-Restart Run Resume Via Provider Session Id](../../08-Task/done/Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md), [BUG-249: Corrupted Builtin Mirror Recreate Duplicates Row And Drops Overrides](./BUG-249-Corrupted-Builtin-Mirror-Recreate-Duplicates-Row-And-Drops-Overrides.md) (same live-testing session), [CA-248: Generalize Resume Session-Validation Bypass For Placeholder Sessions](../../../change-audit/CA-248-generalize-resume-session-placeholder-bypass.md)
- Replaces: `none`
- Tags: `agent-flow-engine, resume, session-management, restart, regression, data-integrity`

## AI Quick View

### Summary

- Found while the owner manually ran CP-36 Scenario 5 (restart the runner mid-loop) against a real project: after killing and restarting the runner while reviewers were actively running, reopening the flow's own hub chat failed with `session_unavailable` — "session data not found on this machine" — even though the flow's own round/cap/cohort state was correctly persisted.
- Root cause: a flow-engine-driven hub's own first provider turn is deliberately suppressed (CP-42) — it only spawns the flow's entry node and is reinvoked later — so its `provider_session_id` never advances past the synthetic `"thread-<n>"` placeholder assigned at spawn, for as long as the flow is still running. `resumeRun`'s existing bypass for exactly this placeholder-session situation only applied while the run was still resident in memory (`isActiveInMemory` requires `inMemory == true`); after a restart the run is always rebuilt from `sessions.ndjson`, `inMemory` is permanently `false`, and the bypass never fires regardless of status — so `ensureResumeReady` runs unconditionally and `LocateSessionFile` fails against a placeholder id that could never have resolved to a real file to begin with.
- Confirmed via two separate live kill/restart tests against real `sessions.ndjson` data (`run-5804`/`run-5809` and `run-6074`/`run-6079`/`run-6181`/`run-6189`) — in both, the hub run's `provider_session_id` stayed on its spawn-time placeholder in every persisted record, including the "completed" one, despite the flow itself progressing correctly through coder → reviewers.

### Current Ask

- Let a restarted run whose own provider session never advanced past its synthetic placeholder still be reopened (read-only, using whatever transcript is on file), instead of permanently erroring.

### Key Decisions

- `V-1` Generalize the existing bypass by the actual invariant that makes it necessary — the current session id is still the synthetic placeholder, which can never resolve via `LocateSessionFile` regardless of in-memory status — rather than by the incidental "still resident in memory" signal the original fix used.
- `V-2` Exclude Codex from the new bypass: `ensureProviderResumeHandle`'s Codex branch already actively re-discovers a real rollout session id from disk even when the stored id is a placeholder (`DiscoverCodexRolloutSessionID`), so that stronger, self-healing path must still run for Codex rather than being short-circuited.
- `V-3` Extract the three bypass conditions into a single named, directly-unit-testable method (`skipsResumeSessionValidation`) instead of inlining a fourth ad-hoc boolean into `resumeRun`, so the exact restart scenario can be asserted without needing to drive the full HTTP/account-resolution stack.

### Constraints

- Scoped to `resumeRun`'s session-file validation gate; `ensureResumeReady`/`ensureProviderResumeHandle`/`LocateSessionFile` themselves are unchanged — they still run their normal strict checks for every other case (a run whose session was genuinely established but is missing from disk, cross-account resume, Gemini project-config checks).
- Does not add any new provider-side session discovery (e.g. a Claude-CLI equivalent of Codex's rollout-file scan) — the fix accepts "no real session yet" as a legitimate, common state for a flow-engine hub and degrades to read-only reopening rather than trying to manufacture a session that never existed.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_handlers.go` `resumeRun` (~line 778, was the sole call site of the old inline bypass), new `skipsResumeSessionValidation` (~line 778).
- `apps/local-runner/internal/runner/interactive_resume.go` `ensureResumeReady` (~line 529), `LocateSessionFile`/`ensureProviderResumeHandle` (~line 747).
- `apps/local-runner/internal/runner/interactive_service.go` `refreshResumeHandleLocked` (~line 3351, only called post-turn — the reason the placeholder persists for the run's whole in-flight duration) and the CP-42 hub-first-turn-suppression comment (~line 3204-3232).
- Owner-provided evidence: `sessions.ndjson` entries for `run-5804`/`run-5809` and `run-6074`/`run-6079`/`run-6181`/`run-6189`, captured across two separate live kill/restart passes.

## 1. Issue Summary

While manually executing CP-36 Scenario 5 against a real backend, the owner started a Review Loop, let the coder finish and reviewers start running, then killed and restarted the local-runner (the scenario's own documented timing). After restart, reopening the run from the desktop (clicking the hub/main agent to view its own chat) failed with `session_unavailable` — the transcript could not be shown at all, even though the flow's own round/cohort/cap state was intact in `sessions.ndjson`.

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Scenario 5 ("Server Restart Mid-Loop") — found while executing that scenario's own documented repro steps.
- impacted design precedent: CP-42's hub-first-turn suppression (a flow-engine-driven hub's own first provider call is skipped; only the entry node spawns) is the reason the hub's `provider_session_id` legitimately has nothing real to advance to until later — this bug is about the resume path not tolerating that already-intentional state after a restart.

## 3. Environment and Reproduction

- environment: Desktop app + local-runner against a real project, Chat Mode Bug sub-mode → Review Loop, Claude provider.
- reproduction steps:
  1. Start a Review Loop run; wait for the coder to complete and at least one reviewer to be spawned and running.
  2. Kill the local-runner process, then restart it.
  3. In the desktop app, click into the hub/main run (the top-level chat that started the flow) to view its transcript.
  4. Observe: `session_unavailable` — "session data not found on this machine."
- frequency: deterministic for any flow-engine-driven hub run restarted before its own first real provider turn (the reinvoke after cohort join) has happened — confirmed identically across two independent live test passes.

## 4. Expected vs Actual

- expected (CP-36 Scenario 5's own intent): after a restart, the desktop reconnects and the board/run remains inspectable, reflecting the correctly-persisted flow state.
- actual: the hub run's own chat panel could not be opened at all post-restart — a hard `session_unavailable` error, not merely a "flow state stale until reinvoke" cosmetic gap.

## 5. Impact

- users affected: anyone restarting the runner (crash, update, manual restart) while a flow-engine-driven orchestration (Review Loop or any custom flow with a `hub.inline` node) has a round in flight.
- workflows affected: reopening/focusing the hub run's chat in the desktop (`focusAgentRun` → `resumeRun`); the flow's own server-side execution and eventual reinvoke are unaffected (this bug only blocks the UI from showing the hub's transcript, not the flow engine's own progress).
- severity: medium-high — not data loss (the flow state itself is intact and correctly persisted), but the run becomes permanently uninspectable in the UI for as long as its own turn was suppressed, which for many flows spans nearly the whole run.

## 6. Root Cause

- confirmed cause: `resumeRun` (`interactive_handlers.go:778`) computed `isActiveInMemory := inMemory && rs.status != Completed/Failed/Cancelled` to skip strict session-file validation for a run whose turn is genuinely in flight. This requires `inMemory == true` (`s.runs[runID]` still resident). After a restart, `s.runs` is always empty, forcing the `rs == nil` branch to call `loadPersistedRun(runID)` — at which point `inMemory` is permanently `false` for the rest of the function, regardless of the rebuilt run's `status`. The subsequent unconditional `ensureResumeReady(rs)` call then reaches `LocateSessionFile(providerKey, srcHome, sessionID, cwd)` with `sessionID` still equal to the synthetic `"thread-<n>"` placeholder — a string that was never assigned by any real provider CLI and therefore can never match a file on disk, by construction.
- evidence: two independent live `sessions.ndjson` captures. Hub `run-5804`: all 3 persisted records (idle/running/completed) show `"provider_session_id":"thread-5805"` — never replaced, despite `"status":"completed"` in the final record. Hub `run-6074`: same pattern (`"thread-6075"` throughout), while its own coder child (`run-6079`) DID get a real id (`"674851bf-1b16-47d4-a7ce-7ec0b78e46a0"`) once its turn actually completed — confirming the placeholder is specific to the hub's own suppressed turn, not a general persistence failure.

## 7. Fix Strategy

- `F-1` Extract the bypass logic from `resumeRun` into `(*InteractiveService).skipsResumeSessionValidation(rs *interactiveRun, inMemory bool) bool`, preserving the two existing reasons (`isActiveInMemory`, `isReadOnlyGeminiInMemory`) unchanged.
- `F-2` Add a third reason, `hasNoRealSession`: `rs.providerKey != ProviderKeyCodex && strings.HasPrefix(s.resumeSessionID(rs), "thread-")` — true whenever the run's current session id is still the synthetic placeholder, independent of `inMemory`. Codex is excluded because its own `ensureProviderResumeHandle` already performs active rollout-file rediscovery for exactly this placeholder case and should keep running.
- `F-3` `resumeRun` now calls `!s.skipsResumeSessionValidation(rs, inMemory)` as its single gating condition, replacing the three inline booleans.

## 8. Validation

- `V-1` `go build ./...` — clean. `go vet ./internal/runner/` — clean.
- `V-2` New test `TestSkipsResumeSessionValidation` (`interactive_service_test.go`), 4 subcases: (a) Claude placeholder session, rebuilt from disk, status completed → must skip (the exact BUG-250 case, reproducing `run-5804`/`run-6074`); (b) Claude with a real session id, rebuilt from disk → must NOT skip; (c) Codex with a placeholder session, rebuilt from disk → must NOT skip (Codex keeps its own self-heal path); (d) Claude placeholder session, still in memory with a turn in flight → must skip (the pre-existing `isActiveInMemory` reason, confirmed unchanged). All 4 pass.
- `V-3` New integration test `TestResumeRunSucceedsForRebuiltRunWithPlaceholderSession`: seeds a `ProviderSessionState` via a fake `WorkflowStore` with `ProviderSessionID: "thread-6075"`, `Status: Completed`, on a service with **zero provider accounts configured** — if the bypass did not fire, `ensureResumeReady` would have nothing to resolve an account/home from and would certainly fail before ever reaching `LocateSessionFile`. `resumeRun` succeeds, confirming the bypass actually took effect through the real `resumeRun` path, not just the extracted helper in isolation. Passes.
- `V-4` `go test ./internal/runner/` (full package, `-count=1`): 15 pre-existing, environment-specific failures (missing real Codex CLI, Windows-specific home-dir/path assertions, provider-home skill-precedence tests needing real files on this machine) — same count as the established baseline. One additional test, `TestProjectRunHistoryFiltersRunsByProject`, failed on a single run mid-session (an ID-counter ordering assertion) but passed both in isolation and on an immediate `-count=1` full rerun — confirmed as pre-existing test-order flakiness unrelated to this change, not a new regression (the new tests were also run standalone alongside it with no failure).
- `V-5` Not performed: a live restart-and-reopen round trip against the owner's actual project. The rebuilt local-runner binary (`bin/flowpilot.exe`) was recompiled with the fix this session; the owner should retry the Scenario 5 restart-and-reopen sequence to confirm the hub chat now opens read-only instead of erroring.

## 9. Regression Guard

- tests: `TestSkipsResumeSessionValidation` (4 subcases) and `TestResumeRunSucceedsForRebuiltRunWithPlaceholderSession`, both in `interactive_service_test.go`.
- audit checks: `gitnexus_detect_changes()` was not run — GitNexus MCP tools were unavailable in this thread; proceeded via direct code inspection per the `add-new-bug` skill's fallback instruction.

## 10. Follow-Up Document Updates

- upstream docs that must change: [CP-36 Scenario 5](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) gets a note recording this finding and the fix, since the scenario's own repro is what surfaced it.
- notes left unchanged on purpose: CP-42's hub-first-turn suppression itself is correct and intentional — this fix does not touch when or whether the hub's own turn runs, only whether the UI can reopen the run's transcript while that turn is still pending.
