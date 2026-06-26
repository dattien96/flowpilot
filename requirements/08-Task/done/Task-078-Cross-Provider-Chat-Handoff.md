# Task-078: Cross-Provider Chat Handoff

## Metadata

- Document ID: `Task-078`
- Title: `Cross-Provider Chat Handoff`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: [Task-162: Summary-Based Cross-Provider Handoff](Task-162-Summary-Based-Cross-Provider-Handoff.md)
- Related Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [Task-063: Desktop Chat Provider Chip Picker](../done/Task-063-Desktop-Chat-Provider-Card-Picker.md), [Task-067: Desktop Post-Restart Run Resume Via Provider Session ID](../done/Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md), [Task-076: Replay User Prompts On Chat Transcript Resume](../done/Task-076-Replay-User-Prompts-On-Chat-Transcript-Resume.md), [BUG-083: Desktop Chat Resume Replays Composed Prompt Not User Input](../../09-BugFix/done/BUG-083-Desktop-Chat-Resume-Replays-Composed-Prompt-Not-User-Input.md), [Task-157: Feature-Key Accuracy For History Context](Task-157-Improve-Context-Hardness.md), [CP-37: Prompt Context Continuity](../../07-Coding-Plan/done/CP-37-Prompt-Context-Continuity.md)
- Replaces: `none`
- Tags: `desktop, chat, provider-switch, cross-provider, handoff, transcript, local-runner`

## AI Quick View

### Summary

- Allow a user to select another provider after a chat has started, with an explicit confirmation modal explaining that FlowPilot will create a new run rather than resume the old provider session.
- Reconstruct a provider-neutral question/answer transcript from the source run using the existing raw turn log and Claude/Codex transcript loaders.
- Start a new chat run under the selected provider and send one bounded handoff prompt containing the prior visible conversation.
- Preserve the source run unchanged and link the source and target runs with handoff metadata for history and diagnostics.

### Current Ask

- The user-confirmed cross-provider switch is now implemented: the desktop creates a fresh target run, transfers the bounded transcript context, and preserves the source run unchanged.

### Key Decisions

- `T-1` A provider switch never migrates or resumes the source provider session. It always creates a distinct target run and provider session.
- `T-2` The handoff source is the runner's persisted transcript data, not `LastPrompt`, `LastMessage`, or only the desktop's currently rendered timeline.
- `T-3` MVP transfers visible user prompts and assistant messages only. It excludes system/developer frames, FlowPilot prompt reinforcement, reasoning, raw tool payloads, and binary attachments.
- `T-4` Confirmation immediately creates the target run and dispatches a handoff bootstrap turn. The bootstrap response remains visible and should briefly acknowledge the imported context.
- `T-5` Raw history is bounded to 64 KiB of UTF-8 prompt content. If the complete history does not fit, retain the newest complete turns and include an explicit truncation notice.
- `T-6` The confirmation modal does not preview the reconstructed context. It only states that FlowPilot will build a context handoff and start a new run. The exact handoff prompt is visible as the first user turn in the new chat. Context extraction therefore happens once, on confirm, not on modal open.
- `T-7` The target model is not chosen in the modal. The handoff reuses the existing per-provider default-model selection (`pickDefaultModel` in `store.ts`: Codex -> `*4-mini*`, Claude -> `*sonnet*`), the same rule applied when a provider is selected normally.
- `T-8` Cross-provider handoff supports Claude and Codex as the source provider only, because only those have a transcript extractor. Gemini as a source is out of scope for this task (see Open Questions and Out of Scope).

### Constraints

- Do not change provider identity on the existing run.
- Do not copy provider-owned session files between different providers.
- Do not label raw transcript text as an AI-generated summary.
- Do not use the truncated 100-character `lastPrompt` or `lastMessage` fields as handoff source data.
- Do not allow switching while a turn is running, awaiting approval, or being interrupted.
- Existing same-provider account switching and resume behavior must remain unchanged.
- **Keep this task and [Task-157: Feature-Key Accuracy For History Context](Task-157-Improve-Context-Hardness.md) decoupled but complementary — both are needed.** This handoff transfers the **conversation transcript** (what was *said* in the thread); Task-157 injects the **feature/commit change history** (what *changed* in code, plus the CA "why"). The transcript cannot be replaced by commit history (it would lose the live discussion), and commit history cannot be replaced by the transcript (the assistant's claimed actions "may be incomplete or incorrect" per `T-7`, whereas the git ledger is ground truth). Because Task-157 injects on **every** turn at the shared prompt-assembly seam (`runner.go` prompt assembly), the handoff **target** run's first turn already receives the feature history automatically — do **not** add it to the handoff prompt or call Task-157 directly. Share one bounded-prompt-block helper (UTF-8-safe truncation, escaping, omission markers) across both rather than reinventing budgeting.

### Open Questions

- `Q-1` Resolved — yes, but in a later phase, not here. This task ships the **raw floor + the slot** the summary plugs into; raw transfer stays the guaranteed zero-budget fallback (the user often switches *because* the source provider hit its token limit). [Task-162](Task-162-Summary-Based-Cross-Provider-Handoff.md) adds the AI-summary hybrid (summary of older turns + most-recent K turns raw), reusing the rolling summarizer built in [Task-161](Task-161-Per-Feature-Chat-Summary-Timeline.md). No RAG.
- `Q-2` Should the handoff confirmation modal allow editing or excluding individual prior turns before transfer?
- `Q-3` (resolved) Gemini as a source provider is deferred to a future feature; MVP gates it out via `handoff_source_provider_unsupported` (see §4 `T-4`). Gemini remains valid as a target.

### Source Refs

- `SS-11` sections `5.1 Cross-Provider Handoff` and `6 Follow-Up Prompt Behavior`
- `SD-12` section `3.3 Cross-Provider Session Rule`
- `CP-18` section `5.6 Cross-Provider Handoff Example`
- `Task-063 T-5` provider lock after the first chat turn
- `Task-076 T-1` through `T-4` provider-neutral prompt replay
- `apps/desktop-flowpilot/src/components/ChatInput.tsx`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/types/contract.ts`
- `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
- `apps/local-runner/internal/runner/interactive_handlers.go`
- `apps/local-runner/internal/runner/interactive_resume.go`
- `apps/local-runner/internal/runner/transcript_loader.go`
- `apps/local-runner/internal/runner/turn_log.go`

## 1. Goal

Allow a user who is already inside a completed or idle desktop chat to continue the topic with another AI provider.

The switch must be explicit and auditable:

1. the user clicks a different provider chip,
2. FlowPilot shows a confirmation modal,
3. the user confirms the target provider and model,
4. FlowPilot reconstructs the source chat's visible question/answer history,
5. FlowPilot creates a new chat run owned by the target provider,
6. FlowPilot sends one handoff bootstrap prompt to the new provider,
7. the desktop moves to the new run while the original run remains available in history.

This is context transfer between two runs, not provider-session migration.

## 2. Parent Links

- coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `SS-11 section 5.1`, `SD-12 section 3.3`, `CP-18 section 5.6`, `Task-063 T-5`, `Task-076 T-1 through T-4`

Upstream prerequisite (satisfied):

- `SS-11` section 5.2 and Product Rule 8 now state that changing provider inside an existing interactive chat creates a new run via user-confirmed transcript context transfer.
- `SD-12` section 3.4 now carries the design contract for provider-neutral transcript extraction and handoff provenance, scoped separately from the workflow-step case in 3.3.
- `CP-18` (done) records this as Successor Work in section 5.7 without reopening its signed-off checklist, since the interactive-chat path is implemented under this task, not CP-18.
- All amendments preserve the existing rule that live provider sessions never cross provider boundaries.

## 3. Trigger

Task-063 intentionally locked the provider picker once a chat contained timeline events. That rule protected one run from being accidentally continued under another provider.

The current runner now has enough persisted information to support a safe alternative:

- raw user prompts are stored in the per-run turn log,
- Claude transcript JSONL can be converted to provider events,
- Codex rollout JSONL files can be converted to provider events,
- Codex multi-turn rollout order is recorded,
- resumed chats already reconstruct ordered prompt and response events.

The missing capability is a provider-neutral handoff operation that creates a new run and deliberately transfers bounded conversation context after user confirmation.

## 4. Exact Change

- `T-1` Update the governing requirement and design before production implementation. (Done.)
  - Business rule added: `SS-11` section 5.2 and Product Rule 8 — changing provider in an existing chat creates a new chat run.
  - Design contract added: `SD-12` section 3.4 — provider-neutral transcript extraction and handoff provenance.
  - `CP-18` section 5.7 records this as Successor Work without reopening its done checklist.
  - Same-provider account resume under `SD-14` is kept separate from cross-provider handoff.
  - Acceptance criteria for confirmation, source-run preservation, target-run creation, and handoff failure behavior live in section 6 of this task.

- `T-2` Replace the permanent provider-chip lock with a switch request while the chat is idle.
  - Keep provider chips disabled while a turn is in flight, an approval is pending, or interruption cleanup is running.
  - Clicking the active provider remains a no-op once the run has started.
  - Clicking another provider opens the confirmation modal and does not immediately mutate `selectedProvider`.
  - Empty chats continue to switch provider directly without confirmation.

- `T-3` Add a cross-provider confirmation modal.
  - Show source provider, target provider, selected target model, and source run identity.
  - State clearly: "This starts a new chat. The previous provider session cannot be resumed by the new provider."
  - State that FlowPilot will build a bounded context handoff from the visible question/answer history and send it as the first prompt of the new chat, where the user can read it verbatim.
  - Do not preview, summarize, or render the reconstructed context inside the modal. Truncation and attachment-omission outcomes are reported in the new run (handoff prompt notice plus diagnostics), not before confirmation.
  - Target model is the per-provider default (`T-7`); the modal does not offer a model picker in MVP.
  - Actions: `Cancel` and `Start new chat with <Provider>`.
  - Cancel closes the modal without changing provider, run, timeline, or history.

- `T-4` Add a runner handoff-context contract.
  - Add a request scoped to a source chat run and target provider.
  - Suggested endpoint: `POST /client/workflow-runs/{runId}/handoff-context`.
  - Suggested request:

```json
{
  "targetProviderKey": "claude",
  "maxBytes": 65536
}
```

  - Suggested response:

```json
{
  "sourceRunId": "run-source",
  "sourceProviderKey": "codex",
  "targetProviderKey": "claude",
  "prompt": "<bounded handoff prompt>",
  "includedTurnCount": 8,
  "omittedTurnCount": 2,
  "truncated": true
}
```

  - Reject workflow runs and non-chat runs in MVP.
  - Reject source and target providers that are equal; same-provider continuity must use the existing resume path.
  - Reject a source run whose provider has no transcript extractor. MVP supports `claude` and `codex` as source providers only (`seedTranscriptFromDisk` and the loaders return early for any other provider). Use a typed `handoff_source_provider_unsupported` error. Gemini as source is a future feature.

- `T-5` Build a provider-neutral conversation extractor in the runner.
  - Use the in-memory ordered event list when the source run is loaded and complete.
  - Otherwise use the same persisted reconstruction path as `seedTranscriptFromDisk`.
  - Override provider-composed prompts with raw turn-log prompts where available.
  - In-memory raw-prompt assumption: today the live `turn_started` event carries `in.Prompt`, which is the raw user input — the ask_user reinforcement and skill/MCP preamble are composed downstream in the adapter, not in the event (see `interactive_service.go` `startTurn`, where the same `in.Prompt` is also persisted as `turnLogKindPrompt`). The in-memory branch is therefore safe for prompts today. To stay safe if prompt composition is ever moved upstream of the event emit, run the raw turn-log override on the in-memory branch as well, not only on the disk branch.
  - Normalize output into ordered records with `role: user | assistant` and plain text content.
  - Include `turn_started.prompt` as user text.
  - Include visible completed assistant message text.
  - Exclude `turn_started` entries with no prompt, terminal status events, thinking/reasoning, approval events, tool call arguments, raw tool results, system frames, developer frames, and FlowPilot-injected preambles.
  - Collapse duplicate assistant final messages when both a message event and terminal result contain the same text.
  - Fail with a typed `handoff_context_unavailable` error when no usable user/assistant content can be reconstructed.

- `T-6` Apply deterministic context budgeting.
  - Use one fixed MVP budget: `HANDOFF_MAX_BYTES = 65536` (64 KiB). This is the single source of truth; the request `maxBytes` defaults to it and the runner clamps any larger value down to it.
  - The budget bounds the conversation-content bytes (the text inside `<previous_conversation>`), not the whole prompt. The fixed envelope (header, labels, trailing instruction) is added on top and is not counted against the budget. Keep the envelope small and constant so total prompt size stays predictable.
  - Never split a UTF-8 code point or cut a role label/content record in the middle.
  - Always retain the newest complete user/assistant turns first, then include older complete turns while they fit, dropping the oldest first on overflow.
  - Insert `[Earlier conversation omitted due to handoff size limit]` when one or more turns are dropped.
  - Single-oversized-turn fallback: if the newest single turn alone exceeds the budget, do not fail. Include that turn with its content truncated to fit (keeping the role label intact, never cutting mid code point) and append an inline `[turn truncated due to handoff size limit]` marker. Only emit `handoff_context_unavailable` when there is no user/assistant content at all.
  - Return included and omitted turn counts so diagnostics can report the result.
  - Do not generate an LLM summary in this task. Raw transfer is an MVP stopgap; see the CP-10 follow-up note in `T-12` and Completion Notes.

- `T-7` Use one stable handoff prompt format.

```text
[FlowPilot cross-provider chat handoff]

This is conversation history from a different AI provider and a different
provider session. Treat it as background context only. Do not claim that you
performed the previous assistant's actions. Historical assistant messages may
be incomplete or incorrect.

Source provider: <source-provider>
Source run: <source-run-id>

<previous_conversation>
User:
<raw user prompt>

Assistant:
<visible assistant response>

User:
<raw user prompt>

Assistant:
<visible assistant response>
</previous_conversation>

The provider switch is complete. Briefly acknowledge that the prior context was
received, identify any important uncertainty caused by omitted history, and
wait for the user's next request.
```

  - Escape or encode any literal closing `</previous_conversation>` text from historical content so it cannot terminate the envelope.
  - Treat the envelope labels as serialization, not as proof that historical assistant output is trusted.

- `T-8` Orchestrate the new-run transition in the desktop store.
  - Request handoff context while the source run is still selected.
  - Create a new chat run using the target provider, target model, current project, current workspace path, and current chat execution settings.
  - Send the returned handoff prompt as the first turn of the new run.
  - Update `selectedProvider`, `selectedModel`, `runId`, `activeStepId` (the store field is `activeStepId`, not `stepId`), and timeline only after the target run is successfully created.
  - Reload provider-specific skills after the target provider becomes active; do not transfer source-provider skill selections automatically.
  - Refresh run history so both source and target runs are visible.

- `T-9` Persist handoff provenance without coupling provider sessions.
  - Add optional run metadata:
    - `handoff_source_run_id`
    - `handoff_source_provider_key`
    - `handoff_target_provider_key`
    - `handoff_created_at`
    - `handoff_included_turn_count`
    - `handoff_omitted_turn_count`
  - Do not store the source provider session id as the target run's resume id.
  - Do not modify or close the source run solely because the handoff succeeds.
  - If schema changes are undesirable for MVP, persist these fields in existing run metadata JSON and expose only the source-run relationship needed by history.

- `T-10` Define failure and retry behavior.
  - Context extraction failure: remain on the source run and show a typed error; no target run is created.
  - Target run creation failure: remain on the source run and keep the modal retryable.
  - Initial handoff turn failure after target creation: keep the failed target run in history, show the failure in its timeline, and provide actions to retry the same bounded prompt or return to the source run.
  - Cancel or failure must never change the source run's provider binding.
  - Prevent duplicate target runs from double-clicks with two independent layers in the desktop, because the handoff is two server operations (create run, then send the first turn) and `startRun` mints a fresh `runId` each call, so it is not self-deduplicating:
    - Layer 1 (logic guard): a store flag (`handoffInFlight`, scoped to the source run id). The confirm handler returns immediately if the flag is already set, then sets it before calling the runner. Clear it on both success and failure so a failed handoff stays retryable.
    - Layer 2 (UI guard): a full-screen loading overlay that blocks all pointer events on the desktop while the handoff runs, and a disabled confirm button. The overlay is dismissed when the flag clears.
  - Server-side create-run deduplication (an idempotency key on run creation that returns the existing target run id) is a hardening follow-up, not MVP; the desktop guard covers the realistic double-click case without a runner change.

- `T-11` Add security and privacy guards.
  - Transfer only text already visible as user/assistant chat content.
  - Do not transfer environment variables, provider credentials, hidden system prompts, reasoning traces, approval payloads, or raw tool request/response bodies.
  - Do not transfer image bytes in MVP; add a visible modal note when the source conversation contained attachments that will not be included.
  - Log run ids, providers, counts, truncation, and status; do not log the full handoff prompt.

- `T-12` Add focused automated coverage.
  - Runner unit tests for Claude extraction, Codex multi-rollout extraction, raw prompt override, role ordering, duplicate assistant suppression, excluded event types, escaping, UTF-8 budgeting, newest-turn retention, and typed empty-history errors.
  - Runner handler tests for invalid run kind, same-provider target, unsupported provider, missing source run, and successful response metadata.
  - Desktop store tests for cancel, successful create-and-send, context failure, start-run failure, first-turn failure, double-confirm idempotency, provider/model state transition, and source-run preservation.
  - Component tests for idle-only switch availability, disabled state during an active turn, modal wording, target provider/model display, and keyboard focus behavior.
  - End-to-end cases for Codex to Claude and Claude to Codex with at least three prior turns.

## 5. Touched Areas

- files:
  - `requirements/05-System-Specs/SS-11-Workflow-With_Session.md`
  - `requirements/06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md` or a new successor cross-provider handoff SD
  - `requirements/07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md` or a new successor CP
  - `apps/desktop-flowpilot/src/components/ChatInput.tsx`
  - `apps/desktop-flowpilot/src/components/ProviderSwitchConfirmModal.tsx` (new)
  - `apps/desktop-flowpilot/src/state/store.ts`
  - `apps/desktop-flowpilot/src/types/contract.ts`
  - `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
  - `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`
  - `apps/local-runner/internal/runner/interactive_handlers.go`
  - `apps/local-runner/internal/runner/interactive_resume.go`
  - `apps/local-runner/internal/runner/transcript_loader.go`
  - focused desktop and runner test files
- modules: desktop chat provider controls, desktop run state, runner transcript reconstruction, runner HTTP contract, chat history metadata
- routes: proposed `POST /client/workflow-runs/{runId}/handoff-context`
- tables: none required if existing run metadata JSON can store provenance; otherwise a migration must be designed upstream before implementation

## 6. Acceptance Check

- In an empty chat, selecting another provider changes selection immediately without a modal.
- In an existing idle chat, selecting another provider opens the confirmation modal and leaves the current run unchanged until confirmation.
- While a turn or approval is active, provider switching is unavailable.
- Cancel preserves the source provider, run id, timeline, model, and history state.
- Confirming Codex to Claude creates a new Claude run and sends one handoff bootstrap turn containing ordered raw user prompts and visible assistant responses.
- Confirming Claude to Codex produces the equivalent result.
- The target run has a new FlowPilot run id and a new provider session id; it never reuses the source provider session id.
- The source run remains reopenable and resumable under its original provider.
- Hidden system/developer content, FlowPilot prompt reinforcement, reasoning, tool payloads, credentials, and attachment bytes are absent from the handoff prompt.
- The confirmation modal does not render the reconstructed context; the exact handoff prompt appears only as the first user turn of the new run.
- The target run uses the per-provider default model (`pickDefaultModel`) without a model picker in the modal.
- Starting a handoff from a Gemini-source chat is rejected with `handoff_source_provider_unsupported` and creates no new run; Gemini remains selectable as a target.
- A history larger than 64 KiB retains newest complete turns, contains the omission marker, and reports non-zero omitted turn count.
- A single newest turn larger than the budget is included truncated with the per-turn truncation marker rather than failing the handoff.
- Double-clicking confirm (or confirm during an in-flight handoff) creates exactly one target run: the in-flight flag blocks re-entry and the full-screen overlay blocks pointer input until the handoff settles.
- `LastPrompt` and `LastMessage` truncation does not affect transferred content.
- A failed handoff-context request creates no new run.
- A failed target bootstrap turn does not corrupt or rebind the source run.
- Run history shows both runs and can identify that the target originated from the source handoff.
- Targeted Go tests, desktop component/store tests, TypeScript typecheck, and relevant builds pass.
- Manual smoke testing confirms modal focus, cancel, successful switch, failure recovery, and reopening both source and target history items.

### 6.1 Definition of Done (DOD)

All items are true for Task-078:

- [x] **DOD-1:** selecting another provider in an idle chat opens the confirmation modal instead of mutating the live run immediately.
- [x] **DOD-2:** confirming the switch creates a new target run and a new provider session.
- [x] **DOD-3:** the handoff prompt is reconstructed from persisted transcript data and bounded to the 64 KiB raw floor.
- [x] **DOD-4:** same-provider switching keeps using the existing resume path; Gemini-source handoff is rejected in MVP.
- [x] **DOD-5:** the source run remains preserved and reopenable after the handoff.
- [x] **DOD-6:** the desktop store, runner endpoint, and type contracts are covered by tests and typecheck.
- [x] **DOD-7 (single target run):** double-clicking confirm (or confirming during an in-flight handoff) creates exactly one target run — the `providerSwitchLoading` logic guard blocks re-entry and the confirm overlay blocks pointer input until the handoff settles; the flag clears on both success and failure so a failed handoff stays retryable.
- [x] **DOD-8 (privacy + escaping):** the handoff prompt carries only visible user/assistant text — hidden system/developer frames, FlowPilot preambles, reasoning traces, tool-call payloads, credentials, and attachment bytes are excluded; any literal `</previous_conversation>` in historical content is escaped via the shared `promptblock` helper so it cannot terminate the envelope.
- [x] **DOD-9 (bounded floor):** raw packing retains newest-complete turns within 64 KiB, inserts the omission marker with a non-zero omitted count on overflow, and includes a single oversized newest turn truncated (with the per-turn marker) rather than failing.

## 7. Out of Scope

- Migrating or sharing a live provider session across providers.
- Copying Claude session files into Codex homes or Codex rollout files into Claude homes.
- Gemini as a handoff **source** provider (no transcript extractor yet); Gemini remains valid as a **target**.
- LLM-generated summaries, semantic compression, or factual verification of historical assistant responses. This raw-transfer approach is the MVP stopgap; the AI-summary context pipeline in [CP-10: Integrations, Memory & Context Intelligence](../../07-Coding-Plan/done/CP-10-Integrations-Hardening.md) is expected to replace it later (tracked as a CP-10 follow-up item).
- Carrying raw reasoning traces, hidden prompts, tool payloads, credentials, or approval state.
- Carrying image/audio/file attachment bytes to the target provider.
- Automatically selecting a provider based on cost, quota, latency, or model capability.
- Automatically switching provider after quota failure; that is separate from the user-initiated switch defined here.
- Merging source and target timelines into one persisted run.
- Deleting or closing the source run after a successful handoff.
- Cross-provider handoff for workflow-step or subagent runs in MVP.
- Injecting feature/commit change history into the handoff prompt — that is [Task-157](Task-157-Improve-Context-Hardness.md)'s job and arrives automatically on the target run's first turn via the shared per-turn injection seam. This task transfers the conversation transcript only.

## 8. Completion Notes

- result: done
- follow-ups: the summary follow-on in Task-162 can refine quality, but the raw bounded handoff itself is complete.
- upstream docs updated: `SS-11` (section 5.2 + Product Rule 8), `SD-12` (section 3.4), and `CP-18` (section 5.7 Successor Work note) capture the implemented cross-provider path.
