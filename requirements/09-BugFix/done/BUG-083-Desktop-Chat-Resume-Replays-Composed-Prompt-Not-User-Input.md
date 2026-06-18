---
name: BUG-083-Desktop-Chat-Resume-Replays-Composed-Prompt-Not-User-Input
description: On chat resume after app/server restart the prompt bubble replays the full prompt FlowPilot composed for the CLI (raw input + ask_user reinforcement + provider preamble) instead of the user's typed text. Codex is worse — it renders the CLI-injected AGENTS.md/environment_context as an extra prompt bubble and drops every turn after the first, because Codex writes one rollout file per turn and resume loads only the stored session id's file.
metadata:
  type: bugfix
---

## Metadata

- Document ID: `BUG-083`
- Title: Desktop Chat Resume Replays Composed Prompt, Not User Input (Codex Also Drops Turns)
- Phase: `bugfix`
- Status: `done`
- Owner: DatNguyen
- Reviewers: —
- Created: 2026-06-18
- Last Updated: 2026-06-18
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: —
- Related Documents: [Task-076: Replay User Prompts On Chat Transcript Resume](../../08-Task/todo/Task-076-Replay-User-Prompts-On-Chat-Transcript-Resume.md), [Task-067: Desktop Post-Restart Run Resume Via Provider Session ID](../../08-Task/todo/Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md), [BUG-046: Desktop History Replay Loses User Prompts](../done/BUG-046-Desktop-History-Replay-Loses-User-Prompts.md), [BUG-077: Codex Skill Prompt Silently Dropped And Wrong Instruction Wording](../done/BUG-077-Codex-Skill-Prompt-Silently-Dropped-Wrong-Instruction.md), [BUG-080: Desktop Run History Lost On App Restart No Supabase](../done/BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md), [BUG-082: Desktop History Chat Open Fails On Legacy Default Account](../done/BUG-082-Desktop-History-Chat-Open-Fails-On-Legacy-Default-Account.md), [CA-098: Provider Session Portability Spike](../../../change-audit/CA-098-spike-provider-session-portability.md), [CA-101: Fix Chat Resume Composed Prompt And Codex Multi-Rollout](../../../change-audit/CA-101-fix-chat-resume-composed-prompt-and-codex-multi-rollout.md)
- Replaces: —
- Tags: desktop, history, resume, transcript, codex, claude, prompt-display, local-runner, severity-high

## AI Quick View

### Summary

- After restarting the app/server and reopening a chat from history, the **user prompt bubble shows the full prompt FlowPilot composed for the CLI**, not the text the user typed. A live chat shows the clean prompt; the same chat after restart shows the composed prompt.
- The composed prompt = raw user input + the `ask_user` reinforcement suffix (`\n\n---\nComplete the clear, unambiguous parts of the task directly …`), plus — for Codex — the CLI-injected `AGENTS.md` / `RTK.md` / GitNexus `<INSTRUCTIONS>` / `<environment_context>` preamble.
- This happens because resume seeds the transcript from the **provider's own on-disk session file** (Claude JSONL / Codex rollout), where the persisted `user` message is exactly what was sent to the CLI. The loaders return that text verbatim and the desktop renders it verbatim. There is no preserved boundary marking the raw user input.
- Codex is additionally wrong in two ways: (B) the CLI-injected preamble is stored as a `role:"user"` frame and is rendered as a bogus prompt bubble; (C) **every turn after the first is missing** because Codex writes a separate rollout file per turn (distinct session id each), and resume loads only the one file matching the stored session id. Claude appends all turns to one JSONL, so Claude keeps every turn.
- FlowPilot's own final `sessions.ndjson` row for `run-68` is the strongest Defect C evidence: `provider_session_id` stayed pinned to turn 1's rollout id (`019ed9bc-...e6e7`), while `last_prompt` and `last_message` prove turn 2 completed.
- Regression follow-up: the first BUG-083 implementation accidentally weakened the `[BugFix]: chat resume fails when same-home accounts trigger relocation` invariant from commit `16606e6`: a continued/resumed Codex chat could persist the newest per-turn rollout id as the durable resume handle. The final fix keeps the stable stored resume id while logging newer rollout ids only for transcript replay.

### Current Ask

- Fix implemented — see §7 and CA-101. All three defects are resolved in `apps/local-runner/internal/runner/`.
- Achieved: on resume, each prompt bubble shows the user's typed text only (raw prompt from turn log overrides composed prompt from provider file); every Codex turn is displayed (all per-turn rollout files loaded via turn-log session-id chain); CLI-injected context no longer renders as a prompt bubble.
- Regression hardening also implemented: Codex replay always loads the persisted session file as the baseline, then de-dupes/appends sidecar rollout ids; `refreshResumeHandleLocked` logs new rollout ids without replacing the durable resume id.

### Key Decisions

- `V-1` The displayed prompt on resume must equal the raw user input — never the CLI-composed prompt (raw + `askUserReinforcement` + `promptPrep` skill/MCP/instruction assembly). The composed prompt that is *sent to the AI* must not change.
- `V-2` Codex resume must reconstruct the conversation from **all** rollout files belonging to the chat, not just the file named by the stored `providerSessionID`.
- `V-3` CLI-injected context frames (`AGENTS.md` / `<INSTRUCTIONS>` / `<environment_context>` / `<permissions instructions>`) must not render as user prompt bubbles, even though Codex records them with `role:"user"`.
- `V-4` Claude and Codex resume must reach display parity (Claude is currently closer to correct only by accident of its single-file transcript and separate system frame).
- `V-5` The Codex `provider_session_id` persisted in `sessions.ndjson` remains the stable durable resume handle. New per-turn rollout ids are sidecar replay inputs, not replacements for the stored resume id.
- `V-6` Legacy Codex chats that predate the BUG-083 sidecar must still replay the persisted baseline session file even after later turns add sidecar entries.

### Constraints

- Do **not** alter the prompt actually composed and sent to the provider CLI — the `ask_user` reinforcement and provider preamble are intentional execution input.
- Do not change the live-turn path: live `turn_started` already carries the raw `in.Prompt` and renders correctly; the regression is replay-only.
- Must not double-render prompts on a live turn (`hasPendingPrompt` guard in `timelineReducer.ts` already covers this).
- A fix that strips a fixed suffix is brittle because `promptPrep` assembly is variable per turn (skills, required MCPs); a durable raw-prompt record or a robust boundary marker is safer.

### Open Questions

- None for BUG-083 after the regression hardening. A separate follow-up bug will track cross-account opening between distinct active accounts of the same provider.

### Source Refs

- On-disk evidence (Codex, two files for one 2-turn chat, distinct session ids):
  - `C:\Users\dat.nguyen\.codex\sessions\2026\06\18\rollout-2026-06-18T14-58-12-019ed9bc-c469-77b3-80d8-0fae7029d6e7.jsonl` — turn 1 (`hello codex 1` + response `Hello. What would you like me to work on?`)
  - `C:\Users\dat.nguyen\.codex\sessions\2026\06\18\rollout-2026-06-18T14-58-28-019ed9bd-035c-7c73-a471-1904f8b27f0a.jsonl` — turn 2 (`hello codex 2` + response `What do you want to work on in C:\working\flowpilot?`)
- FlowPilot local history metadata evidence: `.flowpilot/chats/sessions.ndjson` final last-wins row for `run-68` has `provider_session_id = 019ed9bc-c469-77b3-80d8-0fae7029d6e7`, `status = completed`, `last_prompt = "hello codex 2"`, and `last_message = "What do you want to work on in `C:\working\flowpilot`?"`.
- On-disk evidence (Claude, one file holds both turns): `C:\Users\dat.nguyen\.claude\projects\C--working-flowpilot\f7011859-d013-432e-8d61-c604757ebbdb.jsonl` — `type:"user"` frames carry `hello claude 1 …` and `hellow claude 2 …`, each with the `---\nComplete the clear …` suffix.
- Source search evidence: `environment_context` appears only in this bug note, not in FlowPilot app/source code, so the Codex `<environment_context>` frame is provider CLI-injected rather than FlowPilot-authored UI data.
- `apps/local-runner/internal/runner/interactive_resume.go` — `seedTranscriptFromDisk` (loads exactly one session file)
- `apps/local-runner/internal/runner/transcript_loader.go` — `loadClaudeTranscriptEvents`, `loadCodexTranscriptEvents`
- `apps/local-runner/internal/runner/claude_event_mapper.go` — `claudeUserPromptText` (returns the user frame verbatim)
- `apps/local-runner/internal/runner/codex_event_mapper.go` — `mapCodexRolloutLine` (lines 149-151, every `role:"user"` → `turn_started{prompt}`), `codexRolloutMessageText`
- `apps/local-runner/internal/runner/session_file_locator.go` — `LocateSessionFile` (Codex match `-<sessionID>.jsonl`, line 29), `DiscoverCodexRolloutSessionID`
- `apps/local-runner/internal/runner/codex_adapter.go` — `askUserReinforcement` (line 50), `preparePrompt` (`req.Prompt + askUserReinforcement`), `promptPrep` override (skill + MCP assembly)
- `apps/local-runner/internal/runner/claude_adapter.go` — `claudeAskUserReinforcement` (line 67), mirroring the Codex suffix for Claude turns
- `apps/local-runner/internal/runner/interactive_service.go` — `startTurn` (line 898 emits raw `in.Prompt`; line 880 stores truncated `lastPrompt`)
- `apps/desktop-flowpilot/src/state/timelineReducer.ts` — lines 142-144 render `turn_started.prompt` verbatim as a `kind:"prompt"` bubble
- `apps/desktop-flowpilot/src/types/contract.ts` — line 270, `turn_started` DTO `prompt?: string`
- Regression root cause and guard:
  - `apps/local-runner/internal/runner/interactive_service.go` — `refreshResumeHandleLocked` must log new Codex rollout ids without replacing `realProviderSessionID`.
  - `apps/local-runner/internal/runner/interactive_resume.go` — `seedTranscriptFromDisk` must include the persisted session id before sidecar ids so legacy chats still replay their baseline rollout.
  - `apps/local-runner/internal/runner/cross_account_resume_test.go` — `TestRestoredCodexRunKeepsStablePersistedResumeIDAndLogsNewRollout`.
  - `apps/local-runner/internal/runner/bug083_test.go` — `TestSeedTranscriptIncludesStoredCodexSessionWhenTurnLogIsPartial`.
- GitNexus note: pre-edit impact analysis was run through the local GitNexus CLI for `refreshResumeHandleLocked`, `seedTranscriptFromDisk`, and `interactiveRun`. No HIGH or CRITICAL risk was reported; the index was refreshed with `npx gitnexus analyze` before commit prep.

## 1. Issue Summary

When a Claude or Codex chat is reopened from the desktop history after the app/server is restarted, the replayed user prompt bubble does not match what the user typed:

- **Both providers:** the prompt bubble contains the full prompt FlowPilot composed for the CLI — the user's text plus the appended `ask_user` reinforcement block ("`--- Complete the clear, unambiguous parts of the task directly …`").
- **Codex only, additionally:** an extra prompt bubble shows the CLI-injected `AGENTS.md` / GitNexus `<INSTRUCTIONS>` / `<environment_context>` preamble; and the entire last turn (`hello codex 2` and its assistant response) is missing.

A live (not-yet-restarted) chat shows the clean prompts. The corruption appears only on resume/replay.

## 2. Parent Links

- impacted coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- direct cause: [Task-076](../../08-Task/todo/Task-076-Replay-User-Prompts-On-Chat-Transcript-Resume.md) introduced replaying user prompts from the provider session file; it assumed the persisted `user` message equals the user's typed prompt. It does not — it equals the composed prompt — and for Codex it also assumed one file per conversation.
- resume engine: [Task-067](../../08-Task/todo/Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md) (DOD-067-010 transcript view, `seedTranscriptFromDisk`).
- prior art: [BUG-046](../done/BUG-046-Desktop-History-Replay-Loses-User-Prompts.md) persisted the **raw** prompt on live `turn_started`; the post-restart replay path does not reuse that raw value.

## 3. Environment and Reproduction

- environment: desktop FlowPilot (Electron), local runner, Supabase not configured, local `.flowpilot/chats/sessions.ndjson` history; Codex (`GPT 5.4 Mini`) and Claude providers.
- reproduction steps:
  1. Start a chat (Codex or Claude). Send two prompts, e.g. `hello codex 1` then `hello codex 2`. Confirm the live timeline shows clean prompts and both responses.
  2. Restart the system (the runner / app-server is re-created, clearing the in-memory `s.runs`).
  3. Reopen the same chat from the left history list.
  4. Inspect the replayed transcript.
- frequency:
  - Defect A (composed prompt shown): 100% on every resumed chat, both providers.
  - Defect B (injected-context bubble): 100% on every resumed Codex chat.
  - Defect C (turns after the first lost): 100% on resumed Codex chats with ≥2 turns.

## 4. Expected vs Actual

- expected: the resumed transcript matches the live transcript — each bubble shows only the user's typed text (`hello codex 1`, `hello codex 2`), every turn and response is present, and no system/CLI-injected context appears as a prompt.
- actual:
  - Claude: both prompts appear but each carries the trailing `--- Complete the clear, unambiguous parts …` reinforcement (composed prompt, not raw input). Both responses present.
  - Codex: the first prompt is preceded/accompanied by the CLI-injected `AGENTS.md`/`<INSTRUCTIONS>`/`<environment_context>` block (rendered as a prompt), the first prompt also carries the reinforcement suffix, and the **second turn (`hello codex 2` + its response) is entirely missing**.

## 5. Impact

- users affected: all desktop users reopening chat history after an app/server restart (the default non-Supabase setup).
- workflows affected: run review, debugging, comparison, and follow-up — a resumed Codex chat misreports the conversation (missing turns) and both providers misrepresent what the user actually asked.
- severity: high — no data is destroyed (the provider files retain everything), but the restored conversation is misleading and incomplete: Codex silently drops completed turns, and both providers show internal prompt scaffolding as if the user typed it.

## 6. Root Cause

Symptom vs confirmed cause are separated below. All three defects are **confirmed** by source inspection plus the on-disk session files cited in Source Refs.

### Defect A — composed prompt replayed instead of raw user input (Claude + Codex)

- confirmed cause: resume seeds the timeline from the provider's on-disk session file. `seedTranscriptFromDisk` (`interactive_resume.go`) picks `loadClaudeTranscriptEvents` / `loadCodexTranscriptEvents`. Those loaders read each `user`-role message and emit `turn_started{prompt:<message text>}`. The persisted message **is the composed prompt** FlowPilot sent to the CLI: `codexAdapter.preparePrompt` returns `req.Prompt + askUserReinforcement` (and the live process adapter's `promptPrep` further prepends skill/MCP/instruction assembly). `timelineReducer.ts:142-144` renders `e.prompt` verbatim.
- evidence: Claude `f7011859….jsonl` line `type:"user"` = `hello claude 1\n\n---\nComplete the clear, unambiguous parts …`; Codex rollout `…019ed9bc…` `[user]` = `hello codex 1  --- Complete the clear …`. The `askUserReinforcement` constant (`codex_adapter.go:50`) matches the suffix in the screenshots exactly.
- additional evidence: `claudeAskUserReinforcement` (`claude_adapter.go:67`) mirrors the Codex suffix and explains why Claude replay shows the same appended reinforcement text.
- why live is correct: `startTurn` emits `EventTurnStarted{Prompt: in.Prompt}` (raw) at `interactive_service.go:898`, so the live bubble is clean. The raw prompt is **not** durably persisted per turn — only an in-memory live event and a 100-char-truncated `rs.lastPrompt` (`interactive_service.go:880`) in `sessions.ndjson`. After restart, replay has no clean source and falls back to the provider file.

### Defect B — CLI-injected context rendered as a prompt bubble (Codex only)

- confirmed cause: the Codex rollout records the CLI-injected turn preamble (`AGENTS.md`, `RTK.md`, GitNexus `<INSTRUCTIONS>`, `<environment_context>`) as a `response_item` with `payload.type=="message"`, `role=="user"`. `mapCodexRolloutLine` maps **every** `role=="user"` message to `turn_started{prompt}` (`codex_event_mapper.go:149-151`), so this preamble becomes an extra bogus prompt bubble.
- evidence: in `…019ed9bc….jsonl` the entries before the real prompt are `[developer] <permissions instructions> …` (dropped) and `[user] # AGENTS.md instructions for C:\working\flowpilot <INSTRUCTIONS> … <environment_context> …` (rendered). This block matches the large preamble in the screenshot.
- why Claude is unaffected: Claude keeps system/`CLAUDE.md` content out of `type:"user"` frames (its `f7011859….jsonl` has no AGENTS.md-style user frame), so `claudeUserPromptText` never emits it.

### Defect C — every turn after the first is missing (Codex only)

- confirmed cause: Codex writes a **separate rollout file per turn**, each with a distinct session id. `seedTranscriptFromDisk` resolves exactly one file via `LocateSessionFile(providerKey, home, sessionID, cwd)`, and for Codex that matches only files ending `-<sessionID>.jsonl` (`session_file_locator.go:29`) returning `newestPath`. The run's stored `providerSessionID` is the first turn's id, so only the first turn's rollout is loaded; later turns (their prompt **and** assistant response) live in other files and are never read.
- evidence: the 2-turn Codex chat produced two rollout files 16 seconds apart — `…019ed9bc…e6e7.jsonl` (turn 1) and `…019ed9bd…f0a.jsonl` (turn 2) — each containing only its own turn. The displayed transcript contains exactly turn 1.
- additional evidence: the final `.flowpilot/chats/sessions.ndjson` row for `run-68` stores `provider_session_id = 019ed9bc-c469-77b3-80d8-0fae7029d6e7` while also storing `last_prompt = "hello codex 2"` and `last_message = "What do you want to work on in `C:\working\flowpilot`?"`. FlowPilot metadata therefore proves turn 2 happened, but the replay locator is pinned to turn 1's rollout file.
- why Claude is unaffected: Claude appends all turns to one JSONL (via `--resume <session_id>`); `f7011859….jsonl` holds both turns, so no turn is lost.

### Defect D — regression: BUG-083 sidecar could replace the stable Codex resume handle

- confirmed cause: the first BUG-083 implementation changed `refreshResumeHandleLocked` to rediscover the newest Codex rollout id after every turn and return it for sidecar logging. Because `sessionStateOf` persists `rs.realProviderSessionID` when it is set, a resumed/continued Codex chat could overwrite the durable `provider_session_id` with a newer per-turn rollout id.
- why this regressed commit `16606e6`: the same-home/cross-account open path relies on a stable stored session id that `LocateSessionFile` can prepare or relocate for the active account. Replacing that id with a later per-turn rollout blurred the distinction between "resume handle" and "transcript replay file list".
- second edge: when any `codex_session` sidecar entries existed, `seedTranscriptFromDisk` loaded only those sidecar ids and skipped the stored `provider_session_id`. Legacy chats created before BUG-083 could therefore lose the baseline rollout during replay after a later turn appended sidecar data.
- evidence: `TestRestoredCodexRunKeepsStablePersistedResumeIDAndLogsNewRollout` proves the stored id remains stable while the newest rollout is logged, and `TestSeedTranscriptIncludesStoredCodexSessionWhenTurnLogIsPartial` proves replay includes the stored baseline plus sidecar ids.

## 7. Fix Strategy

Implemented in this turn (CA-101). Each maps to a defect above.

- `F-1` (Defect A) Display the raw user prompt on replay by persisting each turn's raw `in.Prompt` in a per-run sidecar under `.flowpilot/chats/` and having `seedTranscriptFromDisk` prefer it over provider-file text.
- `F-2` (Defect B) In `mapCodexRolloutLine` / `loadCodexTranscriptEvents`, do not emit a prompt bubble for CLI-injected turn-context frames. Detect them (e.g. content starting with `# AGENTS.md instructions`, `<INSTRUCTIONS>`, `<environment_context>`, or the per-turn duplicate preamble) and skip; only emit prompts for genuine user turns.
- `F-3` (Defect C) On Codex resume, reconstruct from the stable stored session file plus every per-turn rollout id recorded in the turn-log sidecar, de-duped in order.
- `F-4` Scope the source switch precisely: keep the provider transcript as the source for assistant messages, tool/command events, file changes, and approvals — only the **user prompt bubble** switches to the FlowPilot-owned raw record (F-1). Provider execution-prompt text must never be the user bubble.
- `F-5` After F-1..F-4, re-verify Claude/Codex display parity (`V-4`).
- `F-6` (Defect D) Keep `realProviderSessionID` stable once known. Track the latest Codex rollout separately with `lastCodexTurnSessionID` so sidecar replay can grow without changing the persisted durable resume handle.
- `F-7` (Defect D) Initialize `lastCodexTurnSessionID` from the persisted session id on reconstruct; this prevents the baseline rollout from being re-logged as a "new" sidecar id and preserves the resume/replay boundary.

## 8. Validation

- `V-1` ✅ `TestSeedTranscriptUsesRawPromptFromTurnLog` — resumed Claude chat: prompt bubble equals raw user input, not the composed `+askUserReinforcement` text.
- `V-2` ✅ `TestSeedTranscriptLoadsAllCodexRolloutFiles` — resumed Codex chat with 2 turns: both prompts and both assistant responses present in order.
- `V-3` ✅ `TestCodexLoaderFiltersInjectedContextFrame`, `TestCodexLoaderFiltersInstructionsTag` — CLI-injected `AGENTS.md`/`<INSTRUCTIONS>`/`<environment_context>` frames not emitted as prompt bubbles.
- `V-4` (manual — not run this turn) Visual parity between live and resumed transcripts requires end-to-end verification with a running app. The unit-test evidence supports the fix is correct.
- `V-5` (unchanged) Live `turn_started` path unmodified; `hasPendingPrompt` guard in `timelineReducer.ts` unchanged.
- `V-6` (unchanged) `codexAdapter.preparePrompt` / `claudeAdapter` prompt-composition path unchanged; only the replay path and the turn-log sidecar are new.
- `V-7` ✅ (prior evidence) On-disk rollout files and source inspection confirmed the root cause; unchanged by this fix.
- `V-8` ✅ `TestRestoredCodexRunKeepsStablePersistedResumeIDAndLogsNewRollout` — a newer rollout id is written to the sidecar without replacing the stable persisted resume id.
- `V-9` ✅ `TestSeedTranscriptIncludesStoredCodexSessionWhenTurnLogIsPartial` — partial sidecar logs still replay the stored baseline rollout plus later sidecar rollout files.
- `V-10` ✅ `go test ./internal/runner -count=1` — 624 runner tests passed.

## 9. Regression Guard

- tests: `bug083_test.go` covers raw prompt override, Codex multi-rollout replay, injected-context filtering, sidecar persistence, and legacy partial sidecar replay. `cross_account_resume_test.go` covers stable Codex resume-id persistence while logging newer rollout ids.
- desktop (proposed): `timelineReducer.test.ts` replay-sequence test stays green; add a case proving a composed-prompt envelope is not shown once F-1 lands.
- audit checks: ensure the composed prompt sent to the CLI is byte-for-byte unchanged (no regression to execution input). Per repo policy, run GitNexus impact analysis before editing the replay, transcript-loader, session-store, or desktop-timeline symbols a fix would touch.

## 10. Follow-Up Document Updates

- upstream docs that must change:
  - [Task-076](../../08-Task/todo/Task-076-Replay-User-Prompts-On-Chat-Transcript-Resume.md): its Acceptance Check claims Claude/Codex parity and "each user prompt bubble appears … with no duplicates", but (a) it replays the **composed** prompt rather than raw input, (b) Codex renders injected-context as a prompt, and (c) Codex loses turns after the first. Task-076's scope/assumptions ("a user prompt is a provider message whose role is `user`") should be corrected to account for the composed envelope and Codex's per-turn rollout-file model.
  - [Task-067](../../08-Task/todo/Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md): the transcript-view capability (DOD-067-010) should note the Codex multi-rollout reconstruction requirement (Defect C).
- notes left unchanged on purpose: the prompt-composition path (`askUserReinforcement`, `preparePrompt`, `promptPrep`) is intentional and out of scope to change — only its *display on replay* is wrong. No `SS`/`SD`/`CP` business intent changes; this is a replay-display correctness delta.
