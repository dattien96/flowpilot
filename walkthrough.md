# Task-210 Option B + run-536 Grok Resume Review

## Verdict

**NEEDS_FIX**

The main Option B path is largely implemented: Grok can locate/copy session directories, legacy `run-370`-style turn-log ids promote before cross-account resume, same-account turns can pass a promoted real id, and relocation failures block rebind instead of falling back to a fresh session.

The run-536 hotfix is only **partially** correct. `refreshResumeHandleLocked` no longer promotes from workspace-wide disk discovery, but `ensureGrokProviderResumeHandle` still has a pre-turn `discoverGrokSessionDirs` fallback when exactly one session dir exists. That is still a wrong-chat steal path for synthetic Grok runs with no `grok_session` turn-log entry.

GitNexus note: GitNexus MCP tools were not exposed in this Codex session, so I could not run `gitnexus_impact` or `gitnexus_detect_changes`. This review used direct source inspection of only the requested files plus the targeted Grok test suite.

## 1. Plan + Hotfix Match

**Done**

- Grok artifact lookup now resolves `GROK_HOME/sessions/<encoded-cwd>/<session-id>/` and requires `chat_history.jsonl`: `apps/local-runner/internal/runner/session_file_locator.go:56`.
- Grok relocation copies full directories, detects identical existing destinations, rejects conflicting destinations, and removes partial copies on failure: `apps/local-runner/internal/runner/session_file_locator.go:243`.
- Destination path construction validates path-safe ids and ensures the target stays under `targetHome`: `apps/local-runner/internal/runner/session_file_locator.go:571`.
- `isGrokRealSessionID` rejects empty, `thread-*`, separators, `.` / `..`, and absolute/path-derived ids: `apps/local-runner/internal/runner/grok_transcript_loader.go:200`.
- Cross-account resume relocates the durable artifact and Grok turn-log session dirs before rebinding: `apps/local-runner/internal/runner/interactive_resume.go:1151` and `apps/local-runner/internal/runner/interactive_resume.go:1159`.
- Legacy `run-370` shape promotes the latest `grok_session` turn-log id before startTurn reaches the adapter: `apps/local-runner/internal/runner/interactive_resume.go:1312`.
- Same-account follow-up uses `realProviderSessionID` for all providers, including Grok: `apps/local-runner/internal/runner/interactive_service.go:2942`.
- Adapter-level hotfix plumbing is sound: `SendTurn` clears stale `lastSessionID`, `ensureSession` sets it only after successful `session/new`/`session/load`, and `LastGrokSessionID` exposes that value: `apps/local-runner/internal/runner/grok_adapter.go:154`, `apps/local-runner/internal/runner/grok_adapter.go:299`, `apps/local-runner/internal/runner/grok_adapter.go:309`.
- Post-turn refresh obeys the run-536 rule: it promotes only adapter-reported `LastGrokSessionID()` and explicitly does not invent an id from disk: `apps/local-runner/internal/runner/interactive_service.go:3676`.

**Partial / risky**

- The hotfix rule says promotion should come only from adapter `LastGrokSessionID()`, never from `discoverGrokSessionDirs`. That is true post-turn, but false pre-turn. `ensureGrokProviderResumeHandle` still promotes `dirs[0]` from disk when exactly one Grok dir exists for the workspace: `apps/local-runner/internal/runner/interactive_resume.go:1330`.

**Not in scope but noted**

- Drive restore still does not support Grok directory packaging. That is outside the stated acceptance criteria, but it remains a future gap if Grok session sync/restore is expected to use these helpers.

## 2. Findings

### Important: Remaining Disk-Discovery Steal Path

`ensureGrokProviderResumeHandle` falls back to `discoverGrokSessionDirs(accountHome, rs.workspaceCwd)` and promotes the only discovered dir as `realProviderSessionID`: `apps/local-runner/internal/runner/interactive_resume.go:1330`.

This is still unsafe. The original run-536 failure proved that `GROK_HOME + cwd` is not a unique run identity. The "exactly one dir" condition only means one other chat exists in that workspace, not that the current synthetic run owns it.

Concrete bad path:

1. Brand-new Grok chat has only synthetic `thread-*`.
2. First turn fails before the adapter reports a real id, so there is no `grok_session` turn-log entry.
3. Another Grok chat under the same `GROK_HOME/cwd` has exactly one session dir.
4. On later resume/account mismatch, `ensureGrokProviderResumeHandle` promotes that unrelated dir, `prepareCrossAccountResume` copies it, and `sessionStateOf` persists the stolen id.

The current run-536 regression test only calls `refreshResumeHandleLocked`: `apps/local-runner/internal/runner/grok_cross_account_resume_test.go:538`. It does not exercise `ensureResumeReady` / `startTurn` with a synthetic Grok run, no turn log, and one unrelated dir.

Fix: remove the `discoverGrokSessionDirs` promotion fallback from `ensureGrokProviderResumeHandle`, or gate it behind a stronger run-owned signal. For Task-210 + run-536, the safest rule is: Grok real resume handles may come from an existing real persisted id, a `grok_session` turn-log id, or adapter `LastGrokSessionID()` only.

### Minor: Same-Account Follow-Up Test Is State-Only

`TestStartTurnGrokSameAccountFollowUpUsesPromotedRealID` verifies that a pre-populated `realProviderSessionID` is passed to the adapter, which is useful. It does not prove the full first-turn to second-turn sequence where a successful adapter-reported id is persisted, then reused on the next turn. The state-level refresh test covers the first half, but not the integrated sequence.

This is not blocking once the steal path is fixed, but a two-turn integration test would better match acceptance criterion 3.

### Minor: Test Uses Non-UUID Session Ids

Most tests use ids like `sess-a` and `sess-stable`. That matches the current permissive `isGrokRealSessionID` implementation, but live Grok ids are UUID-like. If the team wants tighter validation later, these tests will resist it. Not blocking for the current guide because the primary requirement is path safety, not UUID enforcement.

## 3. Acceptance Criteria

1. **Cross-account continue same Grok chat:** mostly done. Locate/copy/rebind/session-load path is implemented and tested for `run-370` shape.
2. **Legacy run-370 shape:** done. Turn-log `grok_session` promotion happens before startTurn dispatch.
3. **Same-account follow-up uses real id:** mostly done. Runtime request selection is correct; integrated two-turn coverage is still thin.
4. **Failed first turn of NEW chat must not steal another session id:** **not fully satisfied**. Post-turn refresh is fixed, but pre-turn single-dir discovery can still steal.
5. **Path-safe session ids and target under targetHome:** done for the reviewed code paths.
6. **Codex/Claude/Gemini unchanged:** no obvious behavior change in the reviewed diff beyond shared helper additions and Grok-specific branches. Targeted tests did not cover full byte-identical behavior.
7. **No silent fresh-session fallback on relocate failure:** done. Relocation failures return `session_unavailable` before adapter execution.

## 4. Test Gaps

- Add a regression for the remaining steal path: synthetic Grok run, no `grok_session` turn log, same/cross-account resume, exactly one unrelated Grok session dir under the source `GROK_HOME/cwd`; expected result is `session_unavailable`, no `realProviderSessionID`, no account rebind, no copied dir, and no adapter call.
- Add a full two-turn same-account test: first fake Grok turn reports `LastGrokSessionID`, persisted state stores it, second turn receives that real id.
- Add adapter-level `session/load` vs `session/new` assertions if not already covered elsewhere: real id uses `session/load`, synthetic/empty uses `session/new`.
- Keep traversal tests; current coverage for malformed ids is good.

## 5. Scope / Structure

The file set is appropriate for Option B. The code is somewhat spread across locator, transcript loader, resume, service, and adapter, but that matches existing provider abstractions and avoids a Grok-only account-switch bypass.

The main structural smell is policy inconsistency: comments in `refreshResumeHandleLocked` say never promote from workspace discovery, while `ensureGrokProviderResumeHandle` still does exactly that in another phase. That inconsistency is what produced the blocking finding.

## 6. Concrete Fix List For Main Agent

1. Remove the `discoverGrokSessionDirs` fallback from `ensureGrokProviderResumeHandle` for Grok synthetic handles with no turn-log id.
2. Preserve these allowed promotion sources only: already-real persisted id, latest run-owned `grok_session` turn-log id, and adapter-reported `LastGrokSessionID()` after a successful session open.
3. Add the run-536 pre-turn regression described above.
4. Add the integrated two-turn same-account reuse test.
5. Re-run the targeted Grok tests and a broader runner test slice covering existing Codex/Claude cross-account resume helpers.

## Verification

Ran:

```bash
go test ./internal/runner -run 'Test(LocateSessionFileGrok|RelocateSessionFileGrok|EnsureProviderResumeHandleGrok|PrepareCrossAccountResumeGrok|StartTurnGrok|RefreshResumeHandleGrok|IsGrokRealSessionID|LocateAndRelocateGrok)' -count=1
```

Result: `15 passed in 1 package`.
