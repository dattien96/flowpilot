# BUG-295: Claude Session Rotation Loses Turn History On Replay

## Metadata

- Document ID: `BUG-295`
- Title: `Claude provider session silently rotates mid-run; persisted session id sticks to the FIRST segment, losing later turns on restart replay`
- Phase: `bugfix`
- Status: `draft`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-20`
- Last Updated: `2026-07-20`
- Parent Documents: `CP-51-PhaseAB-Timeline-And-Verification-Log`
- Child Documents: `-`
- Related Documents: `BUG-294-Resumed-Agent-Card-Shows-Completed-Suffix-On-Cancelled-Child` (same run-12255/run-12260 repro; found together, independent root causes), `BUG-083` (the Codex analogue of this class, already fixed there as F-3)
- Replaces: `-`
- Tags: `agent-flow-engine, transcript-replay, restart, claude-provider, session-management, regression, severity-high`

## AI Quick View

### Summary

- A child coder run (run-12260) silently rotated Claude provider sessions mid-run: turn 1 ran under session `af68de79-...`, turns 2 and 3 ran under a genuinely different session `19647932-...` (verified directly from both real Claude JSONL files on disk — each is internally self-consistent with its own `sessionId` throughout).
- The persisted `provider_session_id` for the run is `af68de79-...` — the FIRST/oldest segment, not the current/latest one.
- On restart, `seedTranscriptFromDisk` for Claude loads exactly one session file — the one named by the persisted id — so only turn 1's content (3 tool calls + "The edit was denied...") survives replay. Everything from turns 2-3 (the flow-gate remediation retry and durable gate-reprompt: "Found the regression... Fixing it.", "I need you to approve the write permission...", "The write is still blocked...") is silently dropped.
- Root cause is NOT in the replay/transcript-loading code itself, but upstream: `refreshResumeHandleLocked`'s Claude branch resolves the pool's real-session mapping through a stable key (`rs.providerSessionID`, set once at run creation and never updated for Claude) that permanently resolves to the FIRST real session ever discovered — so `rs.realProviderSessionID` (and therefore the persisted `provider_session_id`, via `sessionStateOf`) gets reset back to the first segment after every turn, no matter how many times the underlying Claude CLI conversation has actually rotated forward.
- This is the same class of bug BUG-083's F-3 fixed for Codex (which writes one rollout file per turn and previously only loaded the first) — but Claude has no equivalent per-turn session-id tracking in the turn log, and the existing code comment explicitly (and, per this finding, incorrectly) assumes "all Claude runs: one JSONL holds the full conversation."

### Current Ask

- Confirm (via tracing, not yet done in full) exactly why turn 2 lost `--resume` and rotated to a new session, then decide a fix strategy: either (a) prevent the rotation from happening at all (make the pool key stable so `--resume` is never dropped after turn 1), or (b) mirror BUG-083 F-3 for Claude — track every distinct real Claude session id the run passes through in the turn log, and replay all of them in chronological order on restart, the same way Codex rollout files are handled.

### Key Decisions

- None yet — investigation only, no fix applied per this document.

### Constraints

- Any fix touches core provider-session resume logic shared by every Claude interactive run — high blast radius, must be validated against the full `interactive_service`/`interactive_resume` test suite, not just a narrow slice.
- additive-tests-only: any future fix must add new dedicated tests only; no existing resume/session test may be edited or weakened.
- `-race` cannot currently be run on this machine (no gcc/CGO) — any concurrency-sensitive fix in this area should be flagged for `-race` verification on a machine that has it, before being considered fully validated.

### Open Questions

- Is the rotation exactly a ONE-TIME event per run (turn 1 → turn 2, then stable forever after, as observed in this single repro), or can it recur on later turns under different conditions (e.g. a fresh `rs.providerSessionID`-equivalent reset after some other event)? This document only traces the one observed repro; a fix must verify the general case, not just this instance.
- Does this rotation exist ONLY when moving from a synthetic `thread-N` id to the first real id, or can ANY situation where `resumeSessionID(rs)` starts returning a value that was never used as a `req.ProviderSessionID` before also trigger it (e.g. after a cross-account/cross-machine resume, BUG-272-adjacent scenarios)?
- Should the fix be "stop the rotation" (pool key stability) or "tolerate the rotation" (multi-file replay like Codex), or both? A "stop the rotation" fix is likely cheaper and also fixes live mid-conversation continuity implications (not just replay), but needs careful tracing of `claudeProcKey`/`claudeSessionPool` invariants across cross-account and cross-restart resume paths (BUG-272, Task-210, run-536 class fixes) before being touched, since those are exactly the kind of subtle invariant this pool's key scheme protects elsewhere.

### Source Refs

- run-12255 (parent, project db51ec26-1a0f-4b92-8ceb-b03dc8e9b363), child run-12260 (coder), turns turn-12265 / turn-12283 / turn-12312.
- `.flowpilot/chats/sessions.ndjson` line 232 (persisted `provider_session_id: af68de79-398c-4653-85a1-75c9fd60ac51`, `turn_count: 3`).
- `C:\Users\dat.nguyen\.claude\projects\D--working-gate-sandbox\af68de79-398c-4653-85a1-75c9fd60ac51.jsonl` (17 lines, 2026-07-20T03:40:40Z–03:40:49Z, sessionId `af68de79-...` throughout — turn 1 content only: tool calls + "The edit was denied. Did you want to hold off on this change, or should I retry?").
- `C:\Users\dat.nguyen\.claude\projects\D--working-gate-sandbox\19647932-0e14-43b1-b0f2-895757c7410d.jsonl` (31 lines, 2026-07-20T03:41:18Z–03:41:40Z, sessionId `19647932-...` throughout — turns 2-3 content: "Found the regression: `Add` at calc.go:9 subtracts 156. Fixing it.", "I need you to approve the write permission for `calc.go`...", "The write is still blocked pending your approval...").
- Screenshots: FlowPilot Desktop live coder transcript (full history) vs. post-restart replay (only 2 items survive).

## 1. Issue Summary

During a review-loop remediation cycle, a coder child run (run-12260) went through three turns: an initial attempt, a retry after a denied edit, and a durable gate-reprompt after the write approval remained pending. Live, the user saw the complete transcript across all three turns. After the runner was restarted and the parent chat reopened, the child's OWN replayed transcript showed only two items — the tool-call group and final message from turn 1 — with everything from turns 2 and 3 gone. This is not a display-filtering issue (unlike BUG-293); the underlying Claude session genuinely rotated mid-run, and the replay path has no mechanism to discover or load the second session file.

## 2. Parent Links

- impacted coding plan: `CP-51-PhaseAB-Timeline-And-Verification-Log`
- impacted tech design: Claude provider adapter session pool (`claude_adapter.go`), interactive service turn lifecycle (`interactive_service.go: refreshResumeHandleLocked`, `sessionStateOf`), transcript replay (`interactive_resume.go: seedTranscriptFromDisk`)
- impacted system spec: provider session resume contract (07 / cross-restart resume design), BUG-083 F-3 precedent (Codex per-turn rollout tracking)

## 3. Environment and Reproduction

- environment: local runner `127.0.0.1:4318`, desktop FlowPilot, Claude provider (claude-sonnet), workspace `D:\working\gate-sandbox`, account home `C:\Users\dat.nguyen` (`.claude/projects/D--working-gate-sandbox/`).
- reproduction steps (as observed; general trigger condition not yet fully isolated — see Open Questions):
  1. Start a chat/child run whose first turn is a fresh Claude session (no prior real session id yet).
  2. Let the turn complete normally (Claude reports a real session id, e.g. via `system`/`result` frame).
  3. Send a second turn on the same run (e.g. a retry after a denied file edit, or any subsequent turn) that is expected to `--resume` the just-learned real session.
  4. Inspect the account's `.claude/projects/<encoded-cwd>/` folder: a SECOND session file appears with a distinct UUID and its own fully self-consistent `sessionId`, proving Claude started a brand-new conversation instead of resuming.
  5. Restart the runner and reopen the chat: only the FIRST session file's content replays.
- frequency: at minimum once per run (turn 1 → turn 2 transition, in the observed repro); recurrence on later turns not yet verified either way.

## 4. Expected vs Actual

- expected: a single interactive run's Claude conversation stays within one continuous, resumed session for its entire lifetime (matching the code's own stated assumption: "all Claude runs (one JSONL holds the full conversation)"); OR, if rotation is sometimes unavoidable, the runner tracks every rotation (like Codex) and replays all of them in order after a restart.
- actual: the conversation silently rotates to a new session after turn 1, the persisted state points at the STALE first session forever afterward (not even the latest one), and restart replay shows only that first, oldest segment — an information loss that gets worse the longer/more turns a run has, since only ever the very first turn survives a restart.

## 5. Impact

- users affected: any user whose Claude-provider run spans more than one turn and later restarts the runner and reopens that chat — not limited to review-loop/flow-engine runs; this is a general Claude single-turn-vs-multi-turn resume issue.
- workflows affected: any multi-turn Claude interactive chat (normal chat and flow-engine children alike).
- severity: high — silent, unbounded data loss on restart replay for the common case of a multi-turn Claude conversation; more severe than BUG-293 (which only ever hid one internal orchestration message, not real turns/tool calls/user-visible assistant content).

## 6. Root Cause

- hypothesis: the Claude provider session pool's resume-id lookup uses an unstable/mismatched key across turns, causing a spurious `--resume`-drop on the very first post-first-turn attempt, and then a separate bug in the reconstruct-time "refresh" logic keeps re-deriving the STALE first session as the "current" one forever after, rather than tracking the actual latest rotation.
- confirmed cause (from direct evidence):
  - Two independent, internally self-consistent Claude session files exist for run-12260 (`af68de79-...` for turn 1; `19647932-...` for turns 2-3) — this is definitive proof of an actual mid-run Claude session rotation, not a display artifact.
  - `sessionStateOf` (`interactive_service.go:3013-3017`) persists `ProviderSessionID` preferring `rs.realProviderSessionID` when set.
  - `refreshResumeHandleLocked`'s Claude branch (`interactive_service.go:7108-7119`) looks up `live.pool.realSession(rs.providerSessionID)` FIRST (falling back to `rs.realProviderSessionID` only on a miss). `rs.providerSessionID` (the base field, distinct from `rs.realProviderSessionID`) is set once at run creation to the synthetic FlowPilot id and — unlike the Codex/Grok paths (`ensureProviderResumeHandle`, `ensureGrokProviderResumeHandle`, both of which explicitly reassign `rs.providerSessionID` on resume) — is NEVER reassigned anywhere in the Claude code path (verified: no `rs.providerSessionID =` assignment exists for Claude outside run creation).
  - Because `rs.providerSessionID` never changes, the FIRST lookup in `refreshResumeHandleLocked` always resolves through the SAME stable key, which the pool's `setRealSession` populated exactly once (on turn 1, mapping synthetic-id → `af68de79`). Every subsequent turn's post-turn refresh therefore re-derives `rs.realProviderSessionID = af68de79`, discarding whatever the CURRENT turn's real session id actually was.
  - Because `resumeSessionID(rs)` (`interactive_resume.go:2436-2441`) prefers `rs.realProviderSessionID`, turn 2's `req.ProviderSessionID` passed into `claudeAdapter.SendTurn` was `af68de79` itself (the real id, not the synthetic one). `SendTurn`'s `resumeID := a.pool.realSession(req.ProviderSessionID)` (`claude_adapter.go:95`) looked up the pool using `af68de79` AS A KEY — but the pool only ever stored it as a VALUE (under the synthetic key) — a guaranteed cache miss. `resumeID` came back empty, so `claudeArgs` was built WITHOUT `--resume`, and Claude silently started a brand-new conversation, generating `19647932`.
  - Turn 3 happened to work "correctly" (both turns 2 and 3 landed in the SAME `19647932` file) only because turn 2's `SendTurn` also called `a.pool.setRealSession(req.ProviderSessionID, sid)` (`claude_adapter.go:197`) using `af68de79` as the key — establishing a NEW, self-consistent mapping `af68de79 → 19647932` that turn 3's lookup (again keyed by the stuck `af68de79`) could hit. This is coincidental self-healing of the LIVE conversation, not a fix — the PERSISTED/replay-visible session id is still wrong, because `refreshResumeHandleLocked`'s primary lookup path (keyed by the never-changing `rs.providerSessionID`) still overwrote `rs.realProviderSessionID` back to `af68de79` after every turn, matching what was actually persisted.
- evidence: file contents and `sessionId` fields quoted above; `interactive_service.go:3013-3017,7106-7119`; `claude_adapter.go:93-97,177-201`; `interactive_resume.go:2436-2441`; absence of any Claude-path `rs.providerSessionID =` reassignment (grepped, confirmed only Codex/Grok reassign it).

## 7. Fix Strategy (not yet applied)

Two candidate directions — neither implemented yet, pending a decision and deeper tracing of blast radius:

- `F-1` (prevent the rotation) — make the Claude pool's resume lookup key-stable across turns (e.g. always key `claudeProcKey`/`realSession` by the run's STABLE synthetic id, never by a real session id that later gets passed back in as if it were the lookup key), so `--resume` is never spuriously dropped after the first turn. This would fix both live continuity (no more accidental fresh sessions) and replay (persisted id would track the one true continuous session). Requires careful review of `claudeSessionPool` (`claude_adapter.go`) and cross-account/cross-restart resume paths (BUG-272, Task-210 Option B, run-536/584 fixes) that also touch this pool, since those fixes protect specific invariants that must not regress.
- `F-2` (tolerate the rotation, mirror BUG-083 F-3) — track every distinct real Claude session id a run passes through in the turn log (a new `turnLogKindClaudeSession` entry, analogous to `turnLogKindCodexSession`), and have `seedTranscriptFromDisk`'s Claude branch load ALL of them in recorded order (like the existing Codex branch already does), rather than assuming a single file holds the whole conversation. This does not fix the underlying live rotation (or the "sticks to the wrong file" persistence issue) but does stop the data loss on replay.
- A combined `F-1 + F-2` is likely the most robust outcome: stop the unnecessary rotation where possible, AND make replay resilient to any rotation that still legitimately occurs (e.g. a genuine cross-account/cross-restart resume that intentionally starts a new session).

Before implementing either: trace `claudeSessionPool.realSession`/`setRealSession` fully (all call sites, not just the ones found in this pass) and the cross-account/cross-restart resume tests already covering this pool, to avoid reintroducing a regression those tests were written to prevent.

## 8. Validation

- Not yet performed — no fix applied per this document. Root cause was confirmed by direct inspection of the two real Claude session JSONL files and the persisted `sessions.ndjson` record; no code was changed to reach this conclusion.

## 9. Regression Guard

- tests: none yet. A future fix must add new dedicated tests covering at minimum:
  - a multi-turn Claude run where turn 2 resumes turn 1's real session correctly (no spurious rotation) — regression guard for F-1.
  - a multi-turn Claude run whose session DOES legitimately rotate (e.g. cross-account resume) still replays ALL turns after restart, in order — regression guard for F-2.
  - existing cross-account/cross-restart Claude resume tests must continue to pass unmodified (additive-tests-only).
- alerts: consider a runtime warning/log line when `SendTurn`'s `resumeID` comes back empty for a run whose `resumeSessionID` was non-empty (i.e., an unexpected rotation is about to happen) — would have made this bug visible in logs immediately instead of requiring manual JSONL forensics.
- audit checks: a CA note is required once a fix lands (feature_key `agent-flow-engine`).

## 10. Follow-Up Document Updates

- upstream docs that must change: none until a fix is decided. Once `F-1`/`F-2` is chosen and implemented, the BUG-083 F-3 section and this document's Fix Strategy should cross-reference each other as siblings in the same bug class (Codex per-turn rotation vs. Claude session-pool rotation).
- notes left unchanged on purpose: this document intentionally stops at root-cause confirmation per user direction (write BUG docs first, fix later). The exact trigger condition generalization (Open Questions) is deliberately left open rather than guessed.
