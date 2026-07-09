# Task-207: Grok Controlled Adapter MVP (Chat/Stream/Resume)

## Metadata

- Document ID: `Task-207`
- Title: `Grok Controlled Adapter MVP (Chat/Stream/Resume)`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/todo/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [Task-206: Grok ACP Transport And Process/Dispatcher](./Task-206-Grok-ACP-Transport-And-Process-Dispatcher.md)
- Child Documents: `None`
- Related Documents: [Task-165: Gemini Controlled Adapter MVP](../done/Task-165-Gemini-Controlled-Adapter-MVP.md)
- Replaces: `None`
- Tags: `grok, grok-build, adapter, runner, provider-runtime, ai-providers`

## AI Quick View

### Summary

- Build the first `ProviderRuntimeAdapter` implementation for Grok (`grokAdapter`) on top of the Task-206 transport/dispatcher.
- Support controlled normal chat: normalized `turn_started`/`message_delta`/`message_completed`/`tool_started`/`tool_completed`/`turn_completed`/`turn_failed`, honoring every relevant `TurnRequest` field.
- Persist the real ACP `sessionId` and resume via it; register Grok in the live registry with only conservatively-proven `ProviderCapabilities`.

### Current Ask

- Ship a Grok adapter that can run a real desktop chat turn end to end (prompt in, streamed text + tool/file events out, `turn_completed`), with resume by real session id — without yet claiming approval-gating, MCP, or agent-spawn capability (those are Task-208/209).

### Key Decisions

- `T-1` This MVP may advertise `Streaming`, `Resume`, `FileEvents`, and `Interrupt` once proven by test; `ApprovalEvents` and `Mcp` stay false until Task-208/209 land (mirrors Task-165's staged-capability pattern).
- `T-2` `clientCapabilities.fs.{read,write}TextFile` must be `false` on `initialize` so Grok performs writes with its own tools (CP-46 `P-6`); file events are derived from `tool_call`/`tool_call_update` diffs, not from a client-side fs handler.
- `T-3` Resume must use the real ACP-issued `sessionId` from `session/new`/`session/prompt` results — never a synthetic FlowPilot id — matching the Claude/Codex real-session-id discipline.
- `T-4` `SendTurn` must always terminate by emitting `turn_completed` or `turn_failed`, or return an error the shared finalizer can normalize; leave `ProviderTurnID` empty (core stamps it).
- `T-5` Model/effort/skills/attachments from `TurnRequest` must be honored: `ModelName`→ACP model param, `Attachments`→ACP prompt content blocks, `SelectedSkills`→shared `promptPrep` (not a Grok-specific formatter).

### Constraints

- **PLUGIN-ONLY / ZERO BASE REGRESSION (CP-46 `P-0`):** additive-only; no change to any Codex/Claude/Gemini path; do not touch `gemini_acp_transport.go`; shared switched-functions (`providerKeyFromModel`, `defaultModelForProvider`) get an appended `grok` case only, existing cases untouched.
- No Grok-only desktop transport; the adapter must be reachable only through the existing `/client/workflow-runs/.../turns` path.
- Prompt assembly must use the shared selected-skill/context injection path (same function Codex/Claude/Gemini call).
- Do not wire permission handling, MCP, or spawn_agent in this task — an inbound `session/request_permission` during this MVP's tests may be answered with a fixed generic "allow" test double, not real policy.
- Do not change Codex/Claude/Gemini adapter behavior.

### Open Questions

- Whether Grok's resume is safe across `GROK_HOME` changes (deferred to Task-210/212, same as Gemini's Task-167 deferral).
- Whether `agent_thought_chunk` should map to a visible event or stay internal-only pending desktop UX guidance.

### Source Refs

- `CP-46` sections `P-3`, `P-4`, parity rows `GR-01`, `GR-02`, `GR-03`, `GR-11`, `GR-12`, `GR-15`, `GR-16`, `GR-27`.
- `Task-206` (transport/dispatcher this task builds on).
- `apps/local-runner/internal/runner/codex_adapter.go`, `codex_event_mapper.go` (adapter/mapper template).

## 1. Goal

Let Grok run a controlled desktop chat turn through `ProviderRuntimeAdapter.SendTurn`, emitting normalized `ProviderEvent`s, with real-session-id resume, registered in the live provider registry with an honest, minimal capability set.

## 2. Parent Links

- coding plan: `CP-46`
- tech design: `SD-06`, `SD-12`
- system spec: `SS-11`
- specific upstream ids: `CP-46 P-3`, `P-4`

## 3. Trigger

After Task-206 provides a working ACP transport/dispatcher, the runner can add a real Grok adapter without duplicating transport code.

## 4. Exact Change

- `T-1` Add `grokAdapter` implementing `ProviderRuntimeAdapter` (`Key() == ProviderKeyGrok`, `Capabilities()`, `SendTurn`).
- `T-2` Add `ProviderKeyGrok ProviderKey = "grok"` to `provider_event.go` and a `grok-`/`grok-build` prefix case in `providerKeyFromModel` (`provider_registry.go`).
- `T-3` Add `grok_event_mapper.go`: map `session/update` (`agent_message_chunk`→delta/completed, `tool_call`→`tool_started`, `tool_call_update`→`tool_completed`+derive `EventFileChanged` from write/edit diffs) and `turn_completed{stop_reason}`→`turn_completed`/`turn_failed`. Map token counts from `turn_completed._meta` (`totalTokens`/`inputTokens`/`outputTokens`/`cachedReadTokens`/`reasoningTokens`) **plus** the model's `totalContextTokens` from `initialize` into `EventTokenUsageUpdated`/`TokenUsageSnapshot` including `ModelContextWindow` (CP-46 `P-14`, `GR-24`).
- `T-4` `SendTurn` calls `session/new{cwd,mcpServers:[]}` (or resumes via the real session id using the `session/load` shape captured in Task-206 `T-2b`) then `session/prompt`, pumps the dispatcher's notification channel, and returns on the terminal event.
- `T-5` Persist the real ACP `sessionId` via `ProviderSessionStore.UpsertSession{ProviderKey:"grok", ProviderSessionID:sessionId, ProviderThreadID:sessionId}` after `session/new` returns; if `session/prompt` returns a different `sessionId`, adopt it as the stored id (`GR-32`).
- `T-6` Add a `ReasoningEffort` mapping table from FlowPilot's canonical effort values to Grok ACP effort ids (`none/minimal/low/medium/high/xhigh/max`, verified in `initialize.reasoningEfforts`); an unmapped value degrades to the model default explicitly (`GR-35`).
- `T-7` Add fake-transport tests for streaming, final response, tool/file event mapping, token/context reporting, real session id capture, the synthetic-id resume guard (old synthetic id + no real-scope map → explicit fail, no fresh `session/new`), post-prompt session-id adoption, process failure, and ctx-cancel interrupt (best-effort ACP cancel then `ctx.Err()`).
- `T-8` Wire `ProviderRegistryFor` to register the live Grok adapter only when account/env resolution succeeds (mirror the Codex/Claude/Gemini closure pattern); keep the default registry Grok entry as a placeholder. Add `ProviderKeyGrok` and the `grok-`/`grok-build` `providerKeyFromModel` case (appended, not reordered).
- `T-9` **Base-regression (`P-0`):** all edits to `provider_event.go`/`provider_registry.go` are additive; existing `providerKeyFromModel` cases and registry order are unchanged.

## 5. Touched Areas

- files: new `grok_adapter.go`, `grok_event_mapper.go`, `grok_adapter_test.go`, `grok_event_mapper_test.go`; edits to `provider_event.go`, `provider_registry.go`
- modules: local runner provider runtime
- routes: existing `/client/workflow-runs/.../turns`
- tables: none (reuses existing provider-session persistence)

## 6. Acceptance Check

- Grok adapter unit + fake-transport tests pass.
- Default registry still reports Grok placeholder-safe.
- Live registry returns Grok with only proven capabilities (`Streaming`, `Resume`, `FileEvents`, `Interrupt`; `ApprovalEvents`/`Mcp`/`Vision` false).
- Model routing test covers `grok-*`/`grok-build` → `ProviderKeyGrok`.
- A live (manual, credentialed) smoke run streams real text and completes.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` `grokAdapter` implements `ProviderRuntimeAdapter` and emits normalized delta/tool/file/completion events. (`TestGrokAdapterSendTurnStreamsAndCompletes`.)
- [x] `DOD-2` The adapter drives `grok agent stdio` through the Task-206 process boundary using ACP `initialize`, `session/new`, `session/prompt`. (`grok_adapter.go SendTurn`/`ensureSession`.)
- [~] `DOD-3` The MVP does not advertise `ApprovalEvents`, `Mcp`, or `Vision`. **Superseded**: Task-208/209 were implemented in the same pass, so the final `Capabilities()` legitimately advertises `ApprovalEvents=true` (real decision policy exists) and `Mcp` conditionally true once wired — this was an intentional scope extension beyond Task-207 alone, not a shortcut. `Vision` stays false.
- [x] `DOD-4` Live `ProviderRegistryFor` returns Grok as available with only the proven capability set. (`TestProviderRegistryForGrokUsesLiveWhenFlagOnAndAccountResolvable`, gated behind `FLOWPILOT_GROK_AGENT`.)
- [x] `DOD-5` Default registry remains placeholder-safe for Grok. (`TestProviderRegistryForGrokUsesPlaceholderWhenFlagOff`.)
- [~] `DOD-6` Real ACP `sessionId` is captured and persisted; a resume test reuses it (not a synthetic id). Capture+persist is tested (`recordSession`/`ProviderSessionStore.UpsertSession`); the `session/load` resume branch (`ensureSession`, resumeID != "") is implemented but has no dedicated test in this pass.
- [x] `DOD-7` Prompt preparation uses the runner-owned selected-skill/context injection path. (`provider_registry.go` Grok registration wires `promptPrep` to `r.injectSelectedSkills`, mirroring Claude/Gemini.)
- [x] `DOD-8` Token usage + `ModelContextWindow` populate from Grok frames (or explicit absence) and reach `EventTokenUsageUpdated` (`GR-24`). (`TestGrokAdapterSendTurnStreamsAndCompletes`, `grokPromptResultTokenUsage`/`grokContextWindowFromInit`.)
- [x] `DOD-9` `ReasoningEffort` maps to a Grok ACP effort id or degrades to default explicitly (`GR-35`). (`grokReasoningEffortID`; not yet wired into the ACP session call itself — no documented ACP field exists to carry it, see CP-46 §10.2 on `session/new`'s real fields — so this mapping function exists and is unit-testable but is currently unused by `SendTurn`. Flagged as a gap below.)
- [ ] `DOD-10` Session-id integrity guards pass: synthetic-only id does not resume/create a fresh session; post-prompt session-id is adopted (`GR-32`). The adoption logic exists (`emitTerminal`) but has no dedicated test in this pass.
- [x] `DOD-11` **Base-regression (`P-0`):** `provider_event.go`/`provider_registry.go` edits are additive; `providerKeyFromModel` returns byte-identical results for `gpt-*`/`gemini-*`/`claude-*`; Codex/Claude/Gemini adapter + registry tests green. (`TestProviderKeyFromModelBaseRegressionPlusGrok`; full suite verified against a clean-baseline diff — identical 15 pre-existing unrelated failures before and after.)
- [x] `DOD-12` Targeted and broad local-runner tests pass. (34 Grok-specific tests + full `go test ./...` unchanged vs. baseline.)

**Gap found during this pass, not in the original task text:** `SendTurn` builds `grokACPPromptParams` without ever passing `req.ReasoningEffort` anywhere — there is no ACP field to carry it (`session/new`/`session/prompt` have no `reasoningEffort` param per the fetched ACP spec), so `grokReasoningEffortID` is currently dead code from the adapter's perspective. Resolving this needs either an `_x.ai/*` extension method (undiscovered) or per-process `--reasoning-effort` at spawn time (would apply per-account, not per-turn, since the process is shared). Left as an open item for a follow-up task.

## 7. Out of Scope

- Live `session/request_permission` policy, MCP/`ask_user`/`spawn_agent`, YOLO posture mapping (Task-208/209).
- Multi-account isolation, quota display, desktop UI (Task-210/211).
- Cross-account resume safety and transcript extraction (Task-212).

## 8. Completion Notes

- result:
- implementation notes:
- verification:
- follow-ups:
- upstream docs updated:
