# BUG-075: Desktop Chat Image Context Lost On Next Turn

## Metadata

- Document ID: `BUG-075`
- Title: `Desktop Chat Image Context Lost On Next Turn`
- Phase: `bugfix`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [Task-052: Desktop Chat Image Attachments](../../08-Task/done/Task-052-Desktop-Chat-Image-Attachments.md), [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [Task-052: Desktop Chat Image Attachments](../../08-Task/done/Task-052-Desktop-Chat-Image-Attachments.md)
- Replaces: `none`
- Tags: `desktop, chat, image-attachment, multi-turn, claude-adapter, vision`

## AI Quick View

### Summary

- In normal chat mode, an image attached in turn 1 is visible to the AI for that turn, but the AI has no knowledge of the image in turn 2 onward.
- The Claude adapter relies entirely on Claude CLI's `--resume <sessionId>` for conversation continuity; it does not store or replay prior turns' `PromptAttachment` data at the application level.
- Claude CLI's session files do not reliably persist base64 image content from stream-json input, so `--resume` does not guarantee the image survives into the resumed context.
- The `claudeProcessPool.real` session map is in-memory only; a runner restart between turns loses the real session ID mapping and starts a completely fresh session with no prior context at all.

### Current Ask

- Deferred: document and track the root cause for a future fix. No code change in this turn.
- Future fix direction: persist prior turns' `PromptAttachment` data in the event history (`rs.events`) and re-send them as conversation history blocks on each new turn, rather than relying on `--resume` alone. This mirrors the standard multi-turn vision pattern of the direct Anthropic API.

### Key Decisions

- `D-1` Fix is deferred: the cost of re-transmitting image bytes (base64) on every subsequent turn is a deliberate bandwidth/token trade-off the team decided to accept for now.
- `D-2` The fix, when implemented, must be transparent to the user — no manual re-attachment required.
- `D-3` The application-level history replay approach is preferred over relying on CLI session files because it is provider-agnostic and does not depend on undocumented CLI session-storage behavior.

### Constraints

- Do not change `--resume` session plumbing for text continuity; this bug only affects binary image content.
- The fix must not increase per-turn latency for text-only turns (i.e., re-transmission only happens when prior turns contained attachments).
- Keep the `PromptAttachment` wire shape stable; the fix requires persisting it, not changing its schema.

### Open Questions

- Does Claude CLI's session format store base64 image data at all? Confirming this would determine whether the `--resume` path is fixable or must be replaced entirely.
- Should persisted attachments be cleared from event history after a configurable number of turns or total byte budget to bound context growth?
- Should the fix apply only to the Claude adapter, or should the Codex adapter also receive prior-turn images on each turn?

### Source Refs

- `apps/local-runner/internal/runner/claude_adapter.go` — `SendTurn` at line 168: `writeUserTurn(a.preparePrompt(req), req.Attachments)` — only current-turn attachments passed
- `apps/local-runner/internal/runner/claude_stream.go` — `writeUserTurn` lines 113–134: image content blocks written to stdin for current turn only
- `apps/local-runner/internal/runner/claude_process.go` — `claudeProcessPool.real` map lines 79–108: in-memory only, lost on runner restart
- `Task-052 §9.1, §9.4, §9.8` — implementation guide that defines `PromptAttachment`, `TurnInput.attachments`, and `writeUserTurn` signature

## 1. Issue Summary

In the desktop normal chat mode, when a user attaches an image to a prompt (turn 1), the AI successfully processes and responds to the image. However, when the user sends a follow-up prompt in the same session (turn 2), the AI has no knowledge of the image from turn 1. The image context is silently lost between turns.

The symptom is intermittent ("might have no idea") because whether the AI retains any visual context depends on whether Claude CLI's `--resume` mechanism happened to preserve the image data in the session file — which is not guaranteed.

## 2. Parent Links

- impacted coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- originating task: [Task-052: Desktop Chat Image Attachments](../../08-Task/done/Task-052-Desktop-Chat-Image-Attachments.md)

## 3. Environment and Reproduction

- environment: Desktop app, normal chat mode, Claude provider, any model with Vision capability
- reproduction steps:
  1. Open desktop chat in normal chat mode with a Claude provider account selected.
  2. Attach any image to the composer and send prompt: "What is in this image?"
  3. Observe the AI correctly describes the image in turn 1.
  4. Send a follow-up prompt in the same session: "Describe the colors in the image you just saw."
  5. Observe the AI responds as if it has never seen an image (or gives a generic response).
- frequency: Consistently reproducible; intermittent only in degree (the AI may retain text context from turn 1 but loses the image itself)

## 4. Expected vs Actual

- expected: In a multi-turn chat session, the AI retains awareness of images attached in prior turns for the duration of the session, as it would in any standard multi-turn vision API conversation.
- actual: The AI has no visual context of the image after turn 1. It can only infer from text mentioned in prior turns.

## 5. Impact

- users affected: Any user using desktop normal chat with image attachments (Task-052 feature)
- workflows affected: Multi-turn image analysis conversations; use cases where the user sends an image once and asks several follow-up questions about it
- severity: Medium — the core attachment feature (turn 1) works; only the multi-turn continuity is broken. Users who re-attach the image on each turn are unaffected.

## 6. Root Cause

- hypothesis: Claude CLI's `--resume` does not reliably persist base64 image content from stream-json stdin into the session file, so resumed turns have text history but no image data.
- confirmed cause: Not fully confirmed — this requires inspection of Claude CLI session file format. The code-level gap is confirmed: FlowPilot stores no image data in its own event history and passes only the current turn's `req.Attachments` to `writeUserTurn`. There is no application-level replay of prior-turn images.
- evidence:
  - `claude_adapter.go:168`: `proc.stream.writeUserTurn(a.preparePrompt(req), req.Attachments)` — `req.Attachments` contains only the current turn's attachments; prior turns' images are never re-sent.
  - `claude_process.go:79`: `real map[string]string` — the synthetic-to-real session ID map is in-memory only; lost on runner restart, which causes turn 2 to spawn with no `--resume` at all.
  - `interactive_service.go` event history (`rs.events`): stores `ProviderEvent` records (text, tokens, approvals) but never `PromptAttachment` objects. No data exists to replay images on subsequent turns.
  - Task-052 `D-2` (base64-inline transport decision): images are sent once per turn as base64 on stdin. No provision was made for multi-turn replay in V1.

## 7. Fix Strategy

- `F-1` Persist `PromptAttachment` data alongside the prompt event in `rs.events` (or a parallel attachment log) when a turn is submitted with images.
- `F-2` On each new turn start, scan the event history for prior-turn attachments and reconstruct prior user messages as conversation history blocks (text + image content-block arrays) to prepend before the new user turn.
- `F-3` In `claude_stream.go`, add a `writePriorTurns(history []ConversationTurn)` call (or extend `writeUserTurn`) to emit prior-turn messages before the current user turn when history is non-empty.
- `F-4` Bound the replayed image history by a configurable per-session byte budget or turn count to prevent unbounded context and token cost growth.
- `F-5` Durably persist the `real` session map to Supabase (or load it from the existing `ProviderSessionRecord`) on runner startup so a restart does not lose multi-turn text context either.

## 8. Validation

- `V-1` Send an image in turn 1 and ask a follow-up in turn 2 with no new attachment; verify the AI correctly references the turn-1 image.
- `V-2` Send three turns where only turn 1 has an image; verify the image is referenced correctly in turns 2 and 3.
- `V-3` Send a text-only turn 2 after an image-only turn 1; verify no performance regression (no unnecessary base64 re-transmission when prior turns had no images).
- `V-4` Restart the local runner between turn 1 and turn 2 and verify session continuity (text at minimum; images if F-5 is implemented).
- `V-5` Verify the per-session byte budget cap correctly drops old image attachments when the budget is exceeded, rather than failing the turn.
- `V-6` Existing text-only multi-turn chat behavior is unchanged.

## 9. Regression Guard

- tests: Add a Go integration test in `claude_adapter_test.go` that simulates two turns where turn 1 has an attachment and asserts the replay content written to stdin on turn 2 includes the prior image blocks.
- alerts: none beyond existing turn-failure alerting
- audit checks: Verify `writeUserTurn` call site in `SendTurn` includes prior-turn history blocks after the fix.

## 10. Follow-Up Document Updates

- upstream docs that must change: Task-052 `§8 Completion Notes` and `§9.4` should be updated when the fix lands to reflect that multi-turn image replay is now handled at the application level, not delegated to `--resume`.
- notes left unchanged on purpose: `D-2` (base64-inline transport) in Task-052 remains valid — the fix does not change the wire shape, only when and how many times the bytes are sent.
