# Task-076: Replay User Prompts On Chat Transcript Resume

## Metadata

- Document ID: `Task-076`
- Title: `Replay User Prompts On Chat Transcript Resume`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-18`
- Last Updated: `2026-06-18`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [Task-067: Desktop Post-Restart Run Resume Via Provider Session ID](./Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md), [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md), [Task-071: Cross-Account Chat Resume DOD Checklist](./Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md)
- Replaces: `none`
- Tags: `desktop, history, local-runner, resume, transcript, codex, claude, replay`

## AI Quick View

### Summary

- Task-067 (DOD-067-010) seeds the resumed chat timeline from the provider's on-disk session file so reopening a history chat replays the conversation. Both the Claude JSONL loader and the new Codex rollout loader replay **assistant output only** (messages + tool/command/web-search activity).
- The user's own prompt bubbles do **not** reappear on resume. They were never lost — they are simply not emitted as events: during a live turn the desktop renders the prompt bubble client-side (`store.ts` optimistic push), so neither mapper emits a user-message event.
- The desktop is already wired to render a prompt bubble from a `turn_started` event that carries a `prompt` field (`timelineReducer.ts:142`, guarded by `hasPendingPrompt` against double-render, rendered as the existing `kind: "prompt"` TimelineItem). The live path never populates `prompt`, so that branch is currently dormant — it was pre-wired for exactly this resume case.
- Therefore the fix is **backend-only and replay-only**: the transcript loaders should emit `turn_started{prompt}` for each historical user prompt. No new `ProviderEvent` type, no `contract.ts` union change, no `timelineReducer` change.

### Current Ask

- On chat resume, replay the user's prompt bubbles in their correct position (before each assistant response), for **both** Claude and Codex history chats.
- Do not alter live-turn behaviour or risk double-rendering the prompt bubble on an active turn.

### Key Decisions

- `T-1` Reuse the existing `turn_started.prompt` mechanism rather than introducing a new `user_message` event type. The desktop already renders `kind: "prompt"` from `turn_started{prompt}` and de-dupes via `hasPendingPrompt`. This keeps the change to the Go replay loaders only.
- `T-2` The change is **replay-only**: edit the on-disk loaders (`loadClaudeTranscriptEvents` / `mapCodexRolloutLine`), never the live mappers (`mapClaudeLine`/`mapClaudeUser` stay untouched). The Codex rollout mapper is already replay-only, so editing it directly is safe; the Claude live mapper is shared, so user-prompt emission must live in the loader, not in `mapClaudeUser`.
- `T-3` A user prompt is a provider message whose role is `user` (Codex: `response_item.payload` with `role==="user"`; Claude: a `type:"user"` frame whose content is a string or `text` blocks). Frames carrying `tool_result` blocks are tool completions, not prompts, and must keep mapping to `tool_completed`. `developer`/`system` role messages are not replayed.
- `T-4` Each replayed prompt `turn_started` must carry a unique `ProviderTurnID` (e.g. `replay-prompt-<n>`), because the desktop derives the prompt item id from it (`prompt-${e.providerTurnId}`). An empty turn id would collide across all replayed prompts and break React keys.

### Constraints

- Must not change `ProviderEvent`/`ProviderEventDTO` shape, the SSE contract, or `sessions.ndjson`.
- Must not double-render the prompt during a live turn (the `hasPendingPrompt` guard already covers this for `turn_started{prompt}`).
- Parity: Claude and Codex resume must behave the same after this change.

### Open Questions

- `Q-1` Does the replayed stream currently end in a non-thinking, terminal state? Claude's persisted transcript may not contain a `result` frame and the Codex rollout has no `turn.completed`, so replay may end on `message_completed`/`tool_completed`, which under `timelineReducer.finalize` keeps a trailing "Thinking..." row. Verify during implementation; if confirmed, append a synthetic terminal `turn_completed` at end-of-replay (small loader addition) so a resumed, idle chat shows no spinner. Pre-existing behaviour — confirm before treating as in-scope.
- `Q-2` Claude session JSONL: confirm the typed first user prompt content form (plain string vs `[{type:"text"}]`) on the pinned CLI build; the helper must handle both.

### Source Refs

- `apps/local-runner/internal/runner/transcript_loader.go` — `loadClaudeTranscriptEvents`, `loadCodexTranscriptEvents`
- `apps/local-runner/internal/runner/claude_event_mapper.go` — `mapClaudeUser` (live, do NOT change), `claudeMessageContent`
- `apps/local-runner/internal/runner/codex_event_mapper.go` — `mapCodexRolloutLine` (replay-only; `message` role==`user` currently returns nil)
- `apps/local-runner/internal/runner/interactive_resume.go` — `seedTranscriptFromDisk` (stamps correlation fields)
- `apps/desktop-flowpilot/src/state/timelineReducer.ts` — `turn_started` → `kind:"prompt"` (lines 141-145), `hasPendingPrompt` (lines 68-75)
- `apps/desktop-flowpilot/src/state/store.ts` — optimistic prompt push (lines 352-356)
- `apps/desktop-flowpilot/src/types/contract.ts` — `turn_started` DTO already has `prompt?: string` (line 270)

## 1. Goal

When a user reopens a Claude or Codex chat from history, the replayed transcript shows the full back-and-forth — the user's own prompts AND the assistant's responses/tools — in correct order, matching what a live session looks like. Today only assistant output replays.

## 2. Parent Links

- coding plan: [CP-18](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- tech design: [SD-12](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: Task-067 T-3 (transcript view as a follow-up capability), DOD-067-010 (Claude JSONL → SSE snapshot on resume)

## 3. Trigger

Task-067 delivered transcript replay and Task-076's predecessor work added the Codex rollout loader so Codex history chats are no longer blank. During review of that work the user confirmed the remaining gap: resume replays only assistant responses, not the user's prompts. The user designated full prompt+response replay a **core feature** and asked for a detailed implementation plan before coding.

## 4. Exact Change

- `T-1` Add a Claude replay helper `claudeUserPromptText(raw map[string]any) string` (in `claude_event_mapper.go`, used only by the loader) that returns the typed prompt text from a `type:"user"` frame: handle `message.content` as a plain `string` and as a `[]` of `text` blocks; return `""` when the frame contains any `tool_result` block (those remain tool completions).
- `T-2` In `loadClaudeTranscriptEvents` (`transcript_loader.go`): for each line, when `type=="user"` and `claudeUserPromptText` is non-empty, prepend a `ProviderEvent{Type: EventTurnStarted, ProviderTurnID: "replay-prompt-<n>", Prompt: text}` before the existing `mapClaudeLine` output for that line. Maintain an incrementing `<n>` across the file. `mapClaudeUser`/`mapClaudeLine` are NOT modified (live path untouched).
- `T-3` In `mapCodexRolloutLine` (`codex_event_mapper.go`): for `payload.type=="message"` with `role=="user"`, emit `ProviderEvent{Type: EventTurnStarted, Prompt: codexRolloutMessageText(p["content"])}` when the text is non-empty (currently returns `nil`). Keep `role=="developer"`/system dropped, and keep `role=="assistant"` → `message_completed`.
- `T-4` Stamp a unique `ProviderTurnID` on every replayed prompt `turn_started`. For Claude this is done in the loop (T-2). For Codex, since `mapCodexRolloutLine` is per-line and stateless, assign the turn id in `loadCodexTranscriptEvents` (post-map: walk results, set `ProviderTurnID = fmt.Sprintf("replay-prompt-%d", n)` on each prompt-bearing `turn_started`), OR thread an index counter. Confirm uniqueness so desktop `prompt-${providerTurnId}` ids never collide.
- `T-5` (Pending Q-1) If verification shows the replayed stream ends with a trailing "Thinking..." row, append a single synthetic `ProviderEvent{Type: EventTurnCompleted}` at the end of each loader's output so a resumed idle chat renders without a spinner. Gate this on the verification result; do not add speculatively.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/claude_event_mapper.go` (add `claudeUserPromptText` helper)
  - `apps/local-runner/internal/runner/codex_event_mapper.go` (`mapCodexRolloutLine` user-role branch)
  - `apps/local-runner/internal/runner/transcript_loader.go` (both loaders: emit/stamp prompt `turn_started`)
  - `apps/local-runner/internal/runner/codex_transcript_loader_test.go` (extend: assert user prompt → `turn_started` with text)
  - `apps/local-runner/internal/runner/claude_transcript_loader_test.go` (NEW or extend, if a Claude loader test exists: assert user prompt replay + tool_result still maps to tool_completed)
  - `apps/desktop-flowpilot/src/state/timelineReducer.test.ts` (add a replay-sequence test: `turn_started{prompt}` → prompt bubble precedes assistant)
- modules: local-runner transcript replay; desktop timeline reducer (test only)
- routes: none
- tables: none

## 6. Acceptance Check

- Reopen a Codex history chat with ≥2 user turns → each user prompt bubble appears above its assistant response, in order, with no duplicates.
- Reopen a Claude history chat → same parity.
- Live turn unaffected: send a new prompt during an active chat → exactly one prompt bubble (no double-render); confirm `hasPendingPrompt` dedup holds.
- Frames with `tool_result` (Claude) and `function_call_output` (Codex) still render as tool completions, not prompts.
- `developer`/system messages are not rendered as prompts.
- Go: `go build ./...` clean; new/extended loader tests pass. Desktop: reducer test passes; typecheck clean.
- (If Q-1 confirmed) resumed idle chat shows no perpetual "Thinking..." spinner.

## 7. Out of Scope

- No new `ProviderEvent`/DTO type (`user_message`) — explicitly rejected in favour of `turn_started.prompt`.
- No `contract.ts` union change, no `timelineReducer` rendering change beyond tests.
- Replaying prompt **attachments** (images) on resume — the rollout/JSONL prompt text only; image re-hydration is a separate task.
- Replaying skills/model/reasoning chips per historical turn.
- Workflow-run (non-chat) transcript replay — chat runs only, consistent with Task-067 MVP scope.
- Any change to `sessions.ndjson` or cross-PC/cross-account sync behaviour.

## 8. Completion Notes

- result: `not yet implemented` — this document is the solution-design and coding plan only (written to `todo/`). No code changed in this turn.
- follow-ups: resolve Q-1 (terminal-state/thinking-row) during implementation; consider prompt-attachment replay as a separate task.
- upstream docs updated: none required — this task executes within Task-067's transcript-view capability (T-3) and does not change SS/SD/CP intent.
