# BUG-122: UI-Spawned Children Invisible To Parent Provider Conversation

## Metadata

- Document ID: `BUG-122`
- Title: `UI-Spawned Children Invisible To Parent Provider Conversation`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [BUG-121: UI-Spawn Result Not In Parent Timeline And Lost On Restart](./BUG-121-UI-Spawn-Result-Not-In-Parent-Timeline-And-Lost-On-Restart.md), [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Replaces: `None`
- Tags: `multi-agent, spawn, provider-context, orchestration, persistence, restart, runner`

## AI Quick View

### Summary

- When the parent agent spawns a child via the `spawn_agent` tool (prompt-driven), the call and its result are native turns in the parent's provider conversation, so the parent "remembers" the child.
- When the user spawns a child from the desktop UI, the spawn happens outside any parent turn and is never written to the parent's provider conversation — so asking the parent "which sub-agents did we start?" returns "no sub-agents have been started."
- BUG-121 made UI-spawns visible in the *timeline* and persisted, but the timeline is not the provider's conversation; the LLM still had no knowledge of them.
- Fix injects a system note about UI-spawned children (and their results) into the parent's next provider turn, persisted in `sessions.ndjson` so it survives a restart.

### Current Ask

- Make the parent agent aware of children spawned from the UI, matching the prompt-driven spawn behaviour, including after a server restart.

### Key Decisions

- `V-1` After a UI spawn, the parent's next provider turn prompt must contain a note naming the child agent.
- `V-2` The AI `spawn_agent` tool path must NOT inject the note (it is already in provider history) — no double reporting.
- `V-3` The note is injected once and cleared; subsequent turns must not re-send it.
- `V-4` The user-visible prompt bubble (turn_started) must stay clean — the note is provider-only.

### Constraints

- The runner's event log is in-memory only (`fakeWorkflowStore.AppendEvent`); restart-survival must ride on durable channels: the provider session file (Codex rollout / Claude JSONL) and `sessions.ndjson`.
- Injection must not change the displayed prompt, only the provider-bound prompt.
- The pending buffer must be bounded so a user spawning many children without chatting cannot grow the next prompt without limit.

### Open Questions

- Future: when the orchestration feature lets sub-agents talk to each other, the same buffer mechanism may need to carry inter-agent messages, not just spawn/result notes.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` — `spawnChildRun`, `runTurn` (flush), `emitLocked` (child completion), `composeAgentContextBlock`, `appendPendingAgentContext*`
- `apps/local-runner/internal/runner/interactive_handlers.go` — `handleSpawnAgent` (sets `UIInitiated`)
- `apps/local-runner/internal/runner/agent_orchestrator.go` — `SpawnAgentInput.UIInitiated`
- `apps/local-runner/internal/runner/workflow_store.go`, `local_file_session_store.go`, `interactive_resume.go` — `PendingAgentContext` persistence + restore

## 1. Issue Summary

A child agent started from the desktop "Spawn agent" UI never enters the parent agent's provider conversation. The provider (Claude/Codex) only sees what FlowPilot sends as turn prompts plus its own tool calls. A UI spawn is an out-of-band HTTP call (`spawnChildRun`) that fires the child's own turn but never adds anything to the parent's conversation. So when the user later asks the parent "do you know which sub-agent we started?", the model truthfully answers that none were started — it has no record. The prompt-driven path works only because the `spawn_agent` tool call and its returned result are part of the provider's native history.

## 2. Parent Links

- impacted coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md) — P-2 (single backend spawn path for both AI tool and UI action). This bug refines P-2: parity must include the parent's *provider awareness*, not just the run tree.
- impacted tech design: TBD (multi-agent orchestration SD)
- impacted system spec: TBD

## 3. Environment and Reproduction

- environment: Desktop app, local runner (no Supabase), branch `task/agents`
- reproduction steps:
  1. Start a chat (e.g. Codex "orchestrator").
  2. Spawn one or more agents via the Agents panel "Spawn agent" UI.
  3. Ask the main agent: "do you know which sub-agent we started?"
  4. Observe: "no sub-agents have been started yet."
  5. Compare: trigger a spawn by prompting the AI to call `spawn_agent` → asking again, the main agent correctly lists the child.
- frequency: 100% reproducible

## 4. Expected vs Actual

- expected: The parent agent knows about UI-spawned children (names, providers/models, results) on its next turn, matching the prompt-driven spawn.
- actual: The parent agent has no record of UI-spawned children; only prompt-driven spawns are known.

## 5. Impact

- users affected: Anyone using the UI "Spawn agent" action who then expects the main agent to coordinate or reference its sub-agents.
- workflows affected: All future agent-management/orchestration flows where the parent must track and talk to sub-agents.
- severity: High — blocks the orchestration feature's premise (parent coordinating sub-agents) for UI-initiated spawns.

## 6. Root Cause

- hypothesis: UI spawn does not feed the parent's provider conversation.
- confirmed cause: `spawnChildRun` (the shared spawn path) creates the child and fires the child's turn but writes nothing to the parent's provider session. The provider only ingests turn prompts and its own tool I/O. UI spawn produces neither for the parent. The timeline annotations added in BUG-121 are display-only and are not part of the provider conversation; additionally the in-memory event log (`fakeWorkflowStore.AppendEvent`) is not persisted, so even those annotations are not a durable provider-context source.
- evidence:
  - `fakeWorkflowStore.AppendEvent` stores events in a `map` only (`workflow_store.go`).
  - `seedTranscriptFromDisk` rebuilds the transcript from the *provider's own* session files, not from FlowPilot's event log (`interactive_resume.go`) — which is exactly why the prompt-driven spawn survives restart (its tool I/O is in the provider file) and the UI spawn does not.
  - `runTurn` sends `req.Prompt = in.Prompt` straight to the adapter — there was no hook to add parent-side context.

## 7. Fix Strategy

- `F-1` Add `SpawnAgentInput.UIInitiated` (`json:"-"`), set to true by `handleSpawnAgent`; the AI tool path leaves it false. Stamp the child run's `uiInitiated`.
- `F-2` Maintain a bounded `pendingAgentContext []string` buffer on the parent run. On UI spawn append a "started" note; on a UI-spawned child's completion/failure append a "result"/"failed" note (under `s.mu` in `emitLocked`).
- `F-3` In `runTurn`, prepend `composeAgentContextBlock(pendingAgentContext)` to the provider-bound prompt only (not the displayed `turn_started` prompt), then clear the buffer. The cleared state is persisted by the existing post-turn session snapshot.
- `F-4` Persist the buffer in `ProviderSessionState` → `ndjsonSessionRecord` (`pending_agent_context`) and restore it in `reconstructRun`, so a spawn-then-restart-before-asking still reaches the parent. The normal path also survives restart because once injected, the note is part of the provider session file.

## 8. Validation

- `V-1` `TestUISpawnInjectsContextIntoParentProviderTurn`: UI spawn → parent's next provider prompt contains the system note + child name; displayed prompt stays clean; a second turn does not re-inject.
- `V-2` `TestToolSpawnDoesNotInjectParentContext`: tool-path spawn (UIInitiated=false) → parent prompt carries no note.
- `V-3` `go build ./internal/runner/...` clean; `go vet` clean.
- `V-4` Manual: spawn via UI, ask the main agent which sub-agents were started → it lists them; restart the runner, reopen the chat, ask again → still listed (note rode into the provider session file on the first post-spawn turn; pre-turn restart covered by persisted buffer).

## 9. Regression Guard

- tests: `TestUISpawnInjectsContextIntoParentProviderTurn`, `TestToolSpawnDoesNotInjectParentContext` in `interactive_service_test.go`.
- alerts: None.
- audit checks: buffer is capped at `maxPendingAgentNotes` (50) to bound prompt growth.

## 10. Follow-Up Document Updates

- upstream docs that must change: CP-19 P-2 should note that the unified spawn path also surfaces UI spawns into the parent's provider conversation (not only the run tree / panel).
- notes left unchanged on purpose: The event log remaining in-memory is intentional for now; durable provider-context rides on the provider session file and `sessions.ndjson` instead of persisting the event log.
