# CP-40: Gemini Controlled Adapter Over ACP Transport

## Metadata

- Document ID: `CP-40`
- Title: `Gemini Controlled Adapter Over ACP Transport`
- Phase: `coding_plan`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-27`
- Last Updated: `2026-06-28`
- Parent Documents: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- Child Documents: [Task-164: Gemini ACP Transport Extraction](../../08-Task/done/Task-164-Gemini-ACP-Transport-Extraction.md), [Task-165: Gemini Controlled Adapter MVP](../../08-Task/done/Task-165-Gemini-Controlled-Adapter-MVP.md), [Task-166: Gemini Controlled Tools And Approvals](../../08-Task/done/Task-166-Gemini-Controlled-Tools-And-Approvals.md), [Task-167: Gemini Resume Handoff And Live DOD](../../08-Task/todo/Task-167-Gemini-Resume-Handoff-And-Live-DOD.md)
- Related Documents: [03 - Solution And System Design](../../10-Refactor/New-System/03-Solution-And-System-Design.md), [04 - Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md), [07 — Claude Provider Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md), [04-07 — Phase 7: Providers Capability Packaging](../../10-Refactor/New-System/04-07-Phase7-Providers-Capability-Packaging.md)
- Replaces: `None`
- Tags: `gemini, ai-providers, adapter, acp, local-runner, desktop-chat`

## AI Quick View

### Summary

- FlowPilot already has one provider-neutral desktop chat/runtime abstraction in the runner: `ProviderRuntimeAdapter`, `TurnBridge`, normalized `ProviderEvent`, and provider registry.
- Codex and Claude prove the target shape; Gemini should plug into the same seam, not introduce a parallel runtime.
- The repo already contains legacy Gemini ACP session code in `sessions.go`; this CP treats that ACP path as the base transport to extract and modernize.
- The main engineering task is not prompt execution itself. It is making Gemini behave like Codex and Claude in the controlled runner: chat, approvals, tools, skills, context, agent spawning, summaries, workflow rules, modes, resume, and account isolation.
- Gemini is not "done" when it can answer text. It is done when the same desktop and runner workflows that users rely on for Codex/Claude also pass for Gemini, with unsupported capabilities blocked honestly instead of silently bypassed.

### Current Ask

- Create a detailed, code-grounded implementation plan for a real Gemini adapter so desktop chat can use Gemini through the same controlled runner abstraction as Codex and Claude.

### Key Decisions

- `P-1` Gemini should use the existing `gemini_acp` transport shape first, not a new speculative `gemini -p` stream-json path.
- `P-2` The adapter must implement `ProviderRuntimeAdapter` and emit normalized `ProviderEvent`; no special Gemini-only client flow is allowed.
- `P-3` Legacy Gemini session code in `sessions.go` should be extracted into reusable Gemini transport helpers instead of duplicated.
- `P-4` Capability parity must be proved, not assumed. If ACP cannot surface approval/tool/file events cleanly, Gemini remains partially-capable and the runner/UI must say so explicitly.
- `P-5` Gemini must use the same prompt assembly path as Codex and Claude for selected skills, required MCP/tool instructions, context-summary injection, and feature-history injection.
- `P-6` Gemini must use the same runner-owned approval and question gate as Codex and Claude. YOLO=false must never auto-approve a dangerous action; YOLO=true may auto-approve runner-approved tool actions but must not suppress `ask_user`.
- `P-7` Gemini must support `spawn_agent` through the shared `TurnBridge.SpawnAgent` path before agent parity is claimed.
- `P-8` Gemini must pass normal chat, task mode, bugfix mode, workflow-step mode, summary generation, flow-rule gates, and cross-account checks before the adapter is exposed as fully available.

### Constraints

- Preserve the existing runner ownership model: workflow state, approvals, prompt assembly, and finalizer stay in Go runner.
- Do not regress Codex or Claude paths while extracting shared process/JSON-RPC helpers.
- Keep Gemini account/home handling aligned with existing provider-account discovery and `.gemini/settings.json` conventions.
- The desktop client should need little or no provider-specific logic beyond capability rendering.
- Do not mark a capability true in `ProviderCapabilities` until a unit test or manual live check proves the runner can observe and control that capability.
- Do not add a Gemini-only bypass for approvals, agent spawning, summaries, flow gates, or prompt/context injection.
- Treat Gemini as an additive provider feature. Existing Codex, Claude, and shared runner behavior must not be changed except where a neutral extension point or Gemini registration is required.
- Any touched shared/base file must preserve existing behavior and include regression checks for Codex/Claude paths when the touched code is used by those providers.

### Open Questions

- Does Gemini ACP expose enough structured tool and permission events to claim full controlled-mode parity?
- Is Gemini interrupt best implemented as context cancellation only, or does ACP expose a native cancel/interrupt request?
- Can Gemini sessions be listed/read from a live ACP process, or must resume/history stay on the existing persisted session store only?
- Are Gemini ACP session artifacts portable across `GEMINI_HOME` values, or must FlowPilot implement explicit same-provider account mismatch handling before cross-account resume is allowed?
- Can Gemini MCP registration expose runner-owned `ask_user` and `spawn_agent` tools with stable schemas, or does ACP require a different tool bridge?
- Live probe update: the runner process using the current Codex HOME still initializes Gemini ACP but `session/new` fails with `Gemini API key is missing or not configured`; a probe using `HOME=/Users/tiendat` finds OAuth/config files but `session/new` fails with `UNSUPPORTED_CLIENT`, and the Gemini CLI reports this client is no longer supported for Gemini Code Assist for individuals.

### Source Refs

- `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md` sections `Provider Runtime Gateway`, `Provider-Specific Adapter Shape`, and `Provider Capability Model`
- `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- `requirements/06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md` section `5.3 Gemini Transport`
- `apps/local-runner/internal/runner/provider_registry.go`
- `apps/local-runner/internal/runner/provider_event.go`
- `apps/local-runner/internal/runner/sessions.go`
- `apps/local-runner/internal/runner/yolo_resolver.go`
- `apps/local-runner/internal/runner/agent_orchestrator.go`
- `apps/local-runner/internal/runner/summarizer.go`
- `change-audit/CA-079-claude-yolo-off-auto-approval.md`
- `change-audit/CA-091-skill-injection-order.md`
- `change-audit/CA-105-provider-accounts.md`
- `change-audit/CA-109-deterministic-provider-account-ids.md`
- `change-audit/CA-119-provider-consistent-agent-spawn-prompt.md`
- `change-audit/CA-121-agent-panel-runtime-fixes.md`
- `change-audit/CA-132-context-summary-and-handoff.md`
- `requirements/08-Task/done/Task-052-Desktop-Chat-Image-Attachments.md`
- `requirements/08-Task/done/Task-061-Desktop-Chat-Token-Usage-And-Context-Window.md`
- `requirements/08-Task/done/Task-080-Claude-Account-Quota-Usage-Fetch.md`
- `requirements/09-BugFix/done/BUG-092-History-Resume-Fails-When-Persisted-Provider-Account-ID-Is-Stale.md`
- `requirements/09-BugFix/done/BUG-093-Sync-To-Drive-Fails-With-Stale-Provider-Account-ID.md`
- `requirements/09-BugFix/done/BUG-105-Claude-Provider-Spawn-Agent-MCP-Silent-Degradation.md`
- `requirements/09-BugFix/done/BUG-128-Inconsistent-Agent-Spawn-Prompt-Composition-Across-Providers.md`
- `requirements/09-BugFix/done/BUG-129-Child-Agent-Ignores-Parent-Yolo-And-Requests-Approval.md`

## 1. Goal

Implement Gemini as a real controlled-mode provider adapter for desktop chat and runner-driven workflow turns, using the same provider-neutral abstraction already used by Codex and Claude. The result should let FlowPilot start, stream, resume, and finalize Gemini turns through the runner without reintroducing one-off provider plumbing in the desktop client.

## 2. Input Documents

- [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- [03 - Solution And System Design](../../10-Refactor/New-System/03-Solution-And-System-Design.md)
- [04 - Detailed Coding Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md)
- [07 — Claude Provider Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md)

## 3. Implementation Strategy

- overall approach:
  - Reuse the existing controlled-mode contract in the runner: `ProviderRuntimeAdapter`, `TurnBridge`, `ProviderEvent`, `InteractiveService`, and provider registry.
  - Treat Gemini ACP as the transport substrate because the repo already has JSON-RPC session lifecycle and prompt streaming logic for `gemini_acp` in `sessions.go`.
  - Add Gemini through provider-neutral extension points. Do not rewrite Codex/Claude adapters or alter their runtime behavior.
  - Mirror the successful structure of Codex and Claude:
    - dedicated transport/process code
    - dedicated event mapper
    - provider adapter implementing `SendTurn`
    - live-runner registry enablement
    - focused contract/integration tests
- sequencing logic:
  - First validate Gemini ACP behavior against the installed Gemini CLI and the existing legacy code assumptions.
  - Then extract Gemini transport helpers from `sessions.go` into standalone files before wiring the new adapter.
  - Only after transport validation should the placeholder adapter be replaced in the live registry.
- dependencies:
  - Existing provider-neutral runtime contract in `provider_event.go` and `provider_registry.go`
  - Existing Gemini account discovery and config wiring in `provider_accounts.go`
  - Existing legacy Gemini ACP request/response handling in `sessions.go`

### 3.1 Required Parity Matrix

| Capability | Codex/Claude baseline | Gemini requirement | Validation |
| --- | --- | --- | --- |
| Normal chat | Desktop sends turns through the runner, receives normalized stream events, and persists history. | Gemini uses `/client/workflow-runs/.../turns` through `ProviderRuntimeAdapter.SendTurn`; no desktop-only Gemini transport. | `G-01` |
| Task mode | `changeType=task` and task context flow through the same interactive run. | Gemini accepts task-mode turns with identical prompt/context/finalizer behavior. | `G-02` |
| Bugfix mode | `changeType=bugfix` and bug context flow through the same interactive run. | Gemini accepts bugfix-mode turns and participates in bug-rule gates. | `G-02` |
| Workflow-step mode | `workflow_step_auto` resolves provider from model and keeps orchestration runner-owned. | `gemini-*` and `auto-gemini-*` models route to Gemini without special desktop logic. | `G-03` |
| Streaming and completion | Provider events include deltas, final message, token updates when available, and `turn_completed`. | Gemini ACP updates/results map into the same normalized events and always reach the shared finalizer path. | `G-01`, `G-10` |
| YOLO off | Dangerous actions surface `permission_required`; deny stops the action. | Gemini must expose enough permission/tool intent for the runner to request approval. If not, Gemini cannot be marked full parity. | `G-04` |
| YOLO on | Runner can auto-approve eligible tool actions, but user questions still require UI input. | Gemini uses runner `YoloPosture`; auto-approval must not suppress `ask_user`. | `G-05`, `G-06` |
| Approval UI gate | Desktop approval cards are provider-neutral. | Gemini emits normalized `permission_required` and consumes runner approval responses through `TurnBridge.RequestApproval`. | `G-04` |
| User questions | Claude/Codex ask-user surfaces route to `user_question_required`. | Gemini registers/bridges runner-owned `ask_user` and blocks for desktop answer. | `G-06` |
| Spawn agents | `spawn_agent` routes to `TurnBridge.SpawnAgent`, supports wait true/false, child run history, and agent panel. | Gemini tool invocation must call the same bridge and inherit parent provider/yolo/context rules unless an explicit provider is requested. | `G-07` |
| Skill injection | Selected skills are prepended before user task with exact skill content and multi-skill support. | Gemini prompt prep must call the same selected-skill injector used by Codex/Claude; no provider-specific formatting that drops or reorders skills. | `G-08` |
| Context injection | Feature history, latest change-audit context, and chat summary are injected before the model sees the turn. | Gemini must use the same prompt assembly path and preserve context-summary/handoff blocks. | `G-09` |
| Flow rules | `r-ca`, `r-bug`, `r-task`, and related gates run after provider completion through runner finalization. | Gemini must not bypass `finishTurn`; gate reprompt turns must use Gemini and inherit the same prompt/context assembly. | `G-10` |
| Summary generation | Manual and idle summary generation use provider-specific cheap model support. | Gemini summary generation keeps `gemini-2.5-flash` behavior and can summarize Gemini chats. | `G-11` |
| Cross-account use | Provider account IDs are deterministic from provider plus home/config and active account state. | Gemini supports managed `GEMINI_HOME` accounts, detects active-account mismatch, and proves same-provider cross-account behavior before claiming parity. | `G-12`, `G-14` |
| Resume and history | Saved chat history survives runner restart and provider account changes are handled explicitly. | Gemini persists provider session IDs and either resumes safely or returns typed mismatch/recovery behavior. | `G-12`, `G-15` |
| Cross-provider handoff | Gemini can already be a target in some flows; Codex/Claude have source transcript extractors. | Gemini must add a transcript extractor before Gemini-as-source handoff parity is claimed. | `G-13` |
| Attachments/vision | Capability is advertised only when provider runtime supports it. | Gemini `Vision` stays false until ACP attachment behavior is validated through `TurnRequest.Attachments`. | `G-18` |
| Usage and quota failures | Claude/Codex usage-limit failures are classified separately from login failures and are not retried as recoverable. | Gemini auth, quota, rate-limit, and usage-limit failures must become typed/normalized terminal provider errors with actionable recovery text. | `G-19` |
| Provider account metadata | Provider account panels show supported model, quota/usage, and auth metadata when available and degrade cleanly when not. | Gemini account cards must include all returned model/usage buckets, stale metadata must not block reset accounts, and missing auth must be explicit. | `G-20` |
| External MCP servers | Codex/Claude can access FlowPilot-managed MCP servers such as Google Drive without hiding runner-owned approval/question tools. | Gemini must prove configured external MCP servers are visible in the same turn as FlowPilot tools, or keep MCP capability disabled with diagnostics. | `G-21` |
| Detailed child-agent lifecycle | Spawned children surface graph updates, gates, restart visibility, wait semantics, inherited YOLO, and final messages. | Gemini `spawn_agent` parity must pass the same child-run lifecycle matrix before agent parity is claimed. | `G-22` |
| Drive sync and restore | Codex/Claude chat sync/restore recover stale provider account IDs by locating exact same-provider session evidence. | Gemini synced chats must either restore/sync with the same stale-account recovery guarantees or return typed unsupported state without corrupting history. | `G-23` |
| Token and context reporting | Desktop renders provider token usage/context-window data where available and degrades when unavailable. | Gemini token/context reporting must be surfaced only if ACP provides reliable data; otherwise UI/API must show absence without claiming support. | `G-24` |
| Model and reasoning metadata | Provider model controls use supported-model metadata, display names, and provider-specific reasoning affordances. | Gemini supported models, display labels, aliases, and reasoning-effort behavior must be validated against the settings/provider controls. | `G-25` |
| History replay state | Reopened chats replay resolved approvals, questions, tool rows, child runs, and multi-turn transcripts without re-opening stale gates. | Gemini history replay must render Gemini tool/approval/question rows and resolved states consistently with Codex/Claude. | `G-26` |
| Attachment contract fallback | Codex/Claude vision support uses normalized image payloads, timeline thumbnails, and capability-gated unsupported behavior. | Gemini must keep attachment controls disabled while `Vision=false`; if enabled later, it must implement the full Task-052 payload, preview, timeline, and prompt-preservation contract. | `G-27` |

## 4. Work Breakdown

- `P-1` Validate Gemini ACP contract against the real CLI.
  - Confirm `initialize`, `session/new`, `session/prompt`, streaming updates, final result shape, session id persistence, and resume behavior.
  - Capture the real JSON-RPC payloads/notifications so the adapter is based on observed wire shapes, not old assumptions.
  - Capture permission/tool/question/session-update payloads specifically; text streaming alone is not enough.
  - Decide which `ProviderCapabilities` Gemini can truthfully advertise on day one and record proof for each true capability.

- `P-2` Extract Gemini transport primitives from legacy session runtime.
  - Move Gemini ACP request builders, response readers, and text extraction out of `sessions.go` into Gemini-specific transport files.
  - Keep legacy workflow session behavior working while the new adapter is introduced.
  - Identify any generic JSON-RPC helpers that should be shared with Codex instead of copied.

- `P-3` Build Gemini process/session layer for controlled mode.
  - Introduce a Gemini process handle/pool equivalent to the controlled Codex/Claude layers.
  - Support runner-owned process startup, stdin/stdout wiring, session id tracking, idle lifecycle, and failure propagation.
  - Define how Gemini session reuse works for desktop chat and workflow-step reuse.
  - Respect active Gemini account home/config by setting `GEMINI_HOME` or equivalent env per process.

- `P-4` Build `geminiAdapter` on top of the extracted transport.
  - Implement `ProviderRuntimeAdapter` with `Key`, `Capabilities`, and `SendTurn`.
  - Respect `TurnRequest` inputs: cwd, selected skills, model, reasoning effort, attachments, and YOLO mode.
  - Ensure the adapter emits normalized `ProviderEvent` through the `TurnBridge`, never raw Gemini protocol messages.
  - Ensure `SendTurn` always emits a terminal event (`turn_completed` or `turn_failed`) and returns through shared runner finalization.

- `P-5` Map Gemini transport into normalized event semantics.
  - Add `gemini_event_mapper.go` to translate Gemini updates/results into `turn_started`, `message_delta`, `message_completed`, `turn_completed`, `turn_failed`, and any supported tool/file/approval/question events.
  - If Gemini ACP does not surface one of those categories, record the missing capability explicitly and keep the mapper honest.
  - Reuse existing desktop rendering by preserving the shared event schema.
  - Map provider-side tool start/finish and file-change signals if ACP exposes them; otherwise keep `FileEvents` false.

- `P-6` Implement Gemini YOLO and approval posture.
  - Extend or reuse `YoloPosture` so Gemini has an explicit permission mode instead of borrowing Codex/Claude names.
  - For YOLO=false, dangerous tool/file/shell operations must produce `permission_required` and wait for desktop approval.
  - For YOLO=true, only runner-approved action categories may auto-approve; `ask_user` must still surface as a question card.
  - If Gemini persists approval allowlists, clear or override them per turn so stale approvals cannot recreate the Claude YOLO-off bug from `CA-079`.

- `P-7` Register runner-owned Gemini tools for `ask_user` and `spawn_agent`.
  - Add Gemini MCP/tool registration equivalent to the Claude/Codex runtime tools if ACP supports MCP.
  - Expose `ask_user` with the same schema and response contract used by the desktop question UI.
  - Expose `spawn_agent` or a Gemini-safe alias and route it to `TurnBridge.SpawnAgent`.
  - Support `wait=true`, `wait=false`, child run persistence, agent panel state, and inherited provider/yolo/context defaults.

- `P-8` Preserve exact skill and context injection.
  - Gemini prompt prep must call the same selected-skill injection path used by Codex/Claude.
  - Multi-skill selection must keep exact file content and order before the user task, matching `CA-091`.
  - Required MCP/tool instructions, context-summary blocks, feature-history blocks, and handoff context must be assembled by the shared runner path.
  - No Gemini adapter code should independently reformat or summarize selected skill files.

- `P-9` Keep flow gates and rule commands provider-neutral.
  - Ensure Gemini turns reach the same finalizer that invokes `runFlowGate`.
  - Validate `r-ca`, `r-bug`, `r-task`, and related reprompt flows with Gemini.
  - Gate-repair prompts must run through the Gemini adapter when the original turn used Gemini.
  - Flow-gate violation events must use the existing normalized event shape.

- `P-10` Support all desktop modes.
  - Verify Gemini in normal chat, task mode, bugfix mode, and `workflow_step_auto`.
  - Confirm model routing for `gemini-*` and `auto-gemini-*` still resolves to `ProviderKeyGemini`.
  - Keep desktop provider chips, selected-provider readiness, and yolo controls provider-neutral.

- `P-11` Wire account scoping, registry enablement, and resume.
  - Add a live-runner Gemini registration in `ProviderRegistryFor`.
  - Resolve Gemini accounts through existing provider-account discovery and attach the correct home/config/env.
  - Persist and reload Gemini provider session ids through the same session-state path used by other providers.
  - Validate managed homes such as `~/.geminiHomeN` and explicit `GEMINI_HOME`.
  - If active Gemini account changes, fail with a typed account mismatch or perform a proven safe session relocation; do not silently resume under the wrong account.

- `P-12` Implement Gemini transcript extraction and handoff source support.
  - Add a Gemini session transcript locator/extractor if Gemini stores readable session artifacts.
  - Update handoff source support only after extractor tests prove Gemini chats can be reconstructed.
  - Preserve existing Gemini-as-target behavior while adding Gemini-as-source support.
  - If Gemini source extraction is impossible, keep handoff source disabled and document the specific storage limitation.

- `P-13` Keep summary generation working for Gemini chats.
  - Preserve one-shot summarizer support where Gemini uses `gemini-2.5-flash`.
  - Ensure manual "Gen summary", idle summary generation, and context-summary injection work for chats whose active provider is Gemini.
  - Decide whether summary generation should use the new controlled adapter or existing one-shot prompt executor; keep behavior deterministic either way.

- `P-14` Add tests, operator notes, and fallback policy.
  - Add focused unit and integration tests around transport, event mapping, approvals, resume, and failure behavior.
  - Add regression tests or preserve existing tests for every shared/base file touched by Gemini work so Codex and Claude behavior stays unchanged.
  - Update docs/operator notes for Gemini controlled-mode readiness, known gaps, and required local installation/auth steps.
  - Keep the placeholder path as fallback until the live Gemini adapter passes the required validation bar.

- `P-15` Close provider-parity regressions discovered after Codex/Claude rollout.
  - Audit Gemini against the later Codex/Claude bugfix inventory, not only the original adapter DOD.
  - Add Gemini checks for usage/quota classification, account metadata, external MCP availability, child-agent lifecycle edge cases, Drive sync/restore, token/context reporting, supported-model metadata, history replay state, and attachment fallback behavior.
  - Keep each area capability-gated: if Gemini ACP cannot prove a behavior, document the unsupported state and keep the corresponding capability false.
  - Preserve Codex/Claude regression tests while adding Gemini-specific tests for the same behavior families.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/provider_registry.go`
  - `apps/local-runner/internal/runner/provider_event.go`
  - `apps/local-runner/internal/runner/placeholder_adapters.go`
  - `apps/local-runner/internal/runner/sessions.go`
  - new Gemini files such as:
    - `gemini_adapter.go`
    - `gemini_transport.go`
    - `gemini_event_mapper.go`
    - `gemini_process.go`
    - `gemini_mcp_tools.go`
    - `gemini_transcript_extractor.go`
    - `gemini_adapter_test.go`
- modules:
  - local runner provider runtime
  - desktop chat runner integration
  - provider account resolution
  - workflow session persistence/resume
  - agent orchestration bridge
  - flow-gate finalization
  - summary generation
- database:
  - no new table is expected
  - existing provider-session persistence should be reused for Gemini session ids
- external systems:
  - Gemini CLI / ACP process
  - local `.gemini` config/auth state
  - optional MCP/tooling surfaces if Gemini ACP supports them in controlled mode

## 6. Data or Migration Steps

- schema:
  - no schema change is planned for the first Gemini adapter slice
  - Gemini should reuse existing provider-session/event persistence fields
- data backfill:
  - none required
  - existing historical Gemini workflow session records, if any, should continue to read through current store semantics
- config updates:
  - confirm required Gemini env/config inputs (`GEMINI_API_KEY`, `GEMINI_HOME`, `.gemini/settings.json`) and document the supported resolution order
  - if Gemini controlled mode needs explicit MCP or permission config, add it through runner-owned configuration, not desktop-only flags
  - if Gemini needs per-account MCP config files, generate them under the active Gemini home or a runner-owned temp config so dev/prod worktrees and accounts remain isolated

## 7. Validation Plan

- automated tests:
  - `G-01` Normal chat contract: Gemini streams text deltas, completes with `message_completed` and `turn_completed`, saves history, and persists provider session id.
  - `G-02` Task and bugfix contract: Gemini accepts `changeType=task` and `changeType=bugfix`, preserves mode metadata, and reaches finalizer.
  - `G-03` Workflow model routing: `gemini-*` and `auto-gemini-*` models resolve to `ProviderKeyGemini` and run through `workflow_step_auto`.
  - `G-04` YOLO=false approval: a dangerous operation emits `permission_required`; deny prevents the action and produces a controlled failure/result.
  - `G-05` YOLO=true approval: eligible actions auto-approve only through runner policy; the test proves stale provider approvals cannot bypass runner policy.
  - `G-06` `ask_user`: Gemini asks a blocking question, desktop receives `user_question_required`, answer returns to Gemini, and the turn continues.
  - `G-07` `spawn_agent`: Gemini invokes child agents with `wait=true` and `wait=false`; child runs persist, appear in the agent panel, and inherit context defaults.
  - `G-08` Skill injection: one-skill and multi-skill turns show exact selected skill content before user task, matching the Codex/Claude injector.
  - `G-09` Context injection: latest feature history, chat summary, and handoff context appear in the Gemini prompt assembly path.
  - `G-10` Flow gates: `r-ca`, `r-bug`, and `r-task` run after Gemini completion; violation/repair prompts use Gemini and emit shared gate events.
  - `G-11` Summary generation: manual and idle summaries work for Gemini chats and use the expected Gemini cheap model or controlled replacement.
  - `G-12` Cross-Gemini account: two Gemini accounts/homes can be selected independently; resume either works safely under the right account or returns a typed mismatch with recovery instructions.
  - `G-13` Handoff: Gemini can be a target; Gemini-as-source is enabled only after transcript extractor tests pass.
  - `G-14` Account discovery: `GEMINI_HOME`, default `~/.gemini`, and managed `~/.geminiHomeN` accounts produce deterministic provider account IDs.
  - `G-15` Restart resume: runner restart and desktop reload preserve Gemini history and resume metadata.
  - `G-16` Process failure/interrupt: Gemini process death, malformed JSON-RPC, and user interrupt become normalized failures without wedging the run.
  - `G-17` MCP/tool readiness: required Gemini MCP/tool config is installed before prompt start; missing tool config fails early instead of letting the model hallucinate tools.
  - `G-18` Attachments/vision: `Vision` is advertised only after local-image attachment flow is proven through `TurnRequest.Attachments`.
  - `G-19` Usage/quota classification: Gemini auth, quota, rate-limit, and usage-limit failures are normalized away from misleading login text and treated as terminal when retry cannot help.
  - `G-20` Provider account metadata: Gemini account panels show all returned model/usage buckets, reset stale metadata safely, and degrade cleanly when auth or quota data is unavailable.
  - `G-21` External MCP parity: Gemini can see configured FlowPilot-managed MCP servers such as Google Drive in the same turn as `ask_user`/`spawn_agent`, or the provider reports explicit unsupported MCP state.
  - `G-22` Child-agent lifecycle matrix: Gemini spawn-agent runs surface child graph updates, approval/question gates, restart recovery, `wait=true` blocking, `wait=false` result delivery, inherited YOLO, final child text, and provider-independent prompt composition.
  - `G-23` Drive sync/restore: Gemini chat sync and restore recover stale provider-account ids by exact same-provider session evidence, or return typed unsupported state without corrupting local history.
  - `G-24` Token/context reporting: Gemini token usage and context-window data render only when ACP provides reliable values; absence is explicit and does not regress Codex/Claude display.
  - `G-25` Model/reasoning metadata: Gemini supported models, display labels, aliases, and reasoning-effort controls are validated in desktop/provider settings.
  - `G-26` History replay state: Gemini replay renders resolved approvals/questions/tool rows, child runs, and multi-turn transcripts without reopening stale gates or duplicating rows.
  - `G-27` Attachment contract fallback: Gemini keeps image attachment controls disabled while `Vision=false`; if enabled, it implements the full Task-052 normalized payload, preview, timeline, and prompt-preservation contract.
- manual checks:
  - Start desktop with Gemini selected and run a normal chat, task request, and bugfix request.
  - Toggle YOLO off, request a filesystem/shell change, deny it, then confirm no change happens.
  - Toggle YOLO on, request an allowed operation, and confirm approval is runner-owned.
  - Ask Gemini to call `ask_user` and confirm the question card appears and resumes the turn.
  - Ask Gemini to spawn two child agents, one blocking and one non-blocking, and confirm the agent panel and chat history match Codex/Claude behavior.
  - Run `Gen summary` on a Gemini chat and verify the next turn receives the summary context.
  - Switch between two Gemini accounts/homes and confirm history/resume behavior is correct and explicit.
- failure cases:
  - Gemini binary missing
  - Gemini ACP initialize/session creation failure
  - Gemini MCP/tool config missing or rejected
  - malformed or partial JSON-RPC frames
  - interrupted turn
  - unsupported approval/tool/file event visibility
  - stale session id or resume mismatch
  - active provider account mismatch
  - usage/quota/rate-limit failure reported as login or retried as recoverable
  - external MCP configured but not visible to Gemini
  - child-agent wait/approval/restart state invisible or inconsistent
  - stale provider-account id during Drive sync/restore
  - unsupported attachment sent to Gemini and prompt lost

### 7.1 E2E Test Items

| ID | Area | Provider(s) | Test Item | Expected Result |
| --- | --- | --- | --- | --- |
| `E2E-01` | Basic chat | Gemini | Start a new desktop Gemini chat, send a simple prompt, watch streaming, and restart/reopen the chat. | Text streams, final message persists, and history remains readable after restart. |
| `E2E-02` | Skill injection | Gemini, Codex, Claude | Run one-skill and multi-skill prompts with Gemini, then repeat the same prompt shape with Codex and Claude. | Selected skill content/order is preserved and Gemini uses the shared injector behavior. |
| `E2E-03` | Context summary | Gemini, Codex, Claude | Build a long chat, generate or trigger summary, then continue the conversation. | Summary/context is stored and injected into the next provider turn without provider-specific drift. |
| `E2E-04` | Same-account resume | Gemini | Start Gemini, send a turn, continue the chat in the same runner session, and inspect resume metadata. | Gemini reuses the captured provider-owned session id through scoped mapping. |
| `E2E-05` | Restart resume | Gemini, Codex, Claude | Restart the runner/app and resume existing chats for all three providers. | Codex/Claude resume unchanged; Gemini resumes only when safe or returns explicit unsupported/mismatch behavior. |
| `E2E-06` | Cross-account safety | Gemini, Codex, Claude | Create a chat under account/home A, switch to account/home B, then attempt resume. | No provider silently resumes under the wrong account; Gemini returns explicit mismatch/unsupported recovery when not proven safe. |
| `E2E-07` | Synthetic Gemini resume guard | Gemini | Attempt resume using an old synthetic Gemini id with no same-scope real mapping. | Resume fails explicitly and does not create a fresh unrelated `session/new`. |
| `E2E-08` | Prompt-result session id | Gemini | Run a Gemini turn where ACP returns a different `sessionId` after `session/prompt`. | The prompt-result session id becomes the stored provider session id for future turns. |
| `E2E-09` | Approval gate | Gemini, Codex, Claude | With YOLO off, ask for a filesystem/shell action and deny it. | Desktop approval appears, denial blocks the action, and Codex/Claude behavior remains unchanged. |
| `E2E-10` | YOLO policy | Gemini, Codex, Claude | With YOLO on, request an eligible action and also trigger a user question. | Eligible actions follow runner policy; `ask_user` still surfaces as a question and is not auto-approved. |
| `E2E-11` | Tool event rendering | Gemini, Codex, Claude | Trigger tool start/completion events where supported. | Events render through shared provider-event UI; unsupported Gemini file/tool capabilities remain unadvertised. |
| `E2E-12` | Flow gates | Gemini, Codex, Claude | Run task/bugfix flows that trigger `r-ca`, `r-bug`, and `r-task`. | Gate failures and repair prompts use the original provider and shared finalizer path. |
| `E2E-13` | Summary generation | Gemini, Codex, Claude | Use manual `Gen summary` and idle summary paths. | Summaries persist and inject into later turns; blocked Gemini auth is shown as unsupported instead of success. |
| `E2E-14` | Handoff target | Gemini, Codex, Claude | Handoff from Codex/Claude into Gemini, and between Codex/Claude. | Gemini receives prior context as a target; existing Codex/Claude handoff paths stay green. |
| `E2E-15` | Handoff source | Gemini, Codex, Claude | Attempt handoff from Gemini to Codex/Claude and compare with Codex/Claude source handoff. | Gemini-as-source remains disabled or explicitly unsupported until transcript extraction is proven; Codex/Claude source handoff still works. |
| `E2E-16` | Capability flags | Gemini, Codex, Claude | Inspect provider capabilities in UI/API after the run. | Gemini advertises only proven capabilities; existing Codex/Claude capability flags do not regress. |
| `E2E-17` | Child agent spawn | Gemini, Codex, Claude | Ask each provider to spawn child agents with blocking and non-blocking behavior. | Child runs persist, appear in the agent panel, and parent continuation behavior matches supported provider capability. |
| `E2E-18` | Provider failure recovery | Gemini, Codex, Claude | Force binary/auth/process failure for each provider. | Failures become normalized errors, do not wedge the run, and do not corrupt chat history. |
| `E2E-19` | Usage/quota failures | Gemini, Codex, Claude | Force or simulate quota/rate-limit/auth failures. | Usage limits are not reported as login failures and terminal limits are not retried as recoverable. |
| `E2E-20` | Account metadata | Gemini | Refresh Gemini provider accounts with multiple model/usage buckets and missing/expired auth. | All returned buckets display; missing data degrades explicitly without blocking account reset. |
| `E2E-21` | External MCP | Gemini, Codex, Claude | Configure Google Drive MCP and ask each provider to use it with YOLO on and off. | Gemini either sees FlowPilot-managed MCP tools or reports explicit unsupported state; Codex/Claude remain green. |
| `E2E-22` | Child lifecycle matrix | Gemini, Codex, Claude | Exercise child graph visibility, approval/question gates, restart reopen, `wait=true`, `wait=false`, inherited YOLO, and final result delivery. | Gemini matches supported Codex/Claude semantics before agent capability expands. |
| `E2E-23` | Drive sync/restore | Gemini, Codex, Claude | Sync a chat, regenerate provider-account ids, restore/sync before opening, and reopen history. | Same-provider stale-account recovery works where supported; Gemini returns typed unsupported state if live session evidence is unavailable. |
| `E2E-24` | Token/context UI | Gemini, Codex, Claude | Inspect token usage and context-window display after turns. | Gemini reports real ACP values only; otherwise UI shows unavailable without breaking Codex/Claude. |
| `E2E-25` | Model metadata | Gemini | Edit/select Gemini supported models and aliases, then run chat and workflow-step routing. | Display labels, aliases, and reasoning controls resolve to the expected Gemini model or are disabled explicitly. |
| `E2E-26` | History replay state | Gemini, Codex, Claude | Reopen chats containing tool calls, approvals, questions, child runs, and multiple turns. | Replay shows resolved states correctly and does not duplicate rows or reopen stale gates. |
| `E2E-27` | Attachments fallback | Gemini, Codex, Claude | Try image attachment in Gemini while `Vision=false`, then compare Codex/Claude supported paths. | Gemini blocks or errors before losing the prompt; Codex/Claude attachment paths remain unchanged. |

## 8. Rollout and Fallback

- rollout order:
  - keep Gemini as placeholder in the default registry
  - enable Gemini only in the live-runner registry after `P-1` validation and `P-4` adapter wiring are complete
  - land extraction first, then adapter, then enablement, then desktop/manual validation
- fallback path:
  - if Gemini ACP lacks required structured event surfaces, keep Gemini available only for the legacy prompt-execution/session path and leave controlled-mode desktop chat disabled or hidden behind an explicit partial-capability flag
  - if approval/question/agent semantics are incomplete, expose reduced capabilities instead of faking Codex/Claude parity
  - if cross-account behavior cannot be proven, do not claim account parity; return typed account mismatch behavior rather than silently resuming under the wrong home
  - if extraction from `sessions.go` proves too risky in one slice, first wrap the existing Gemini ACP path, then refactor internals in a follow-up
- monitoring:
  - runner logs around Gemini process lifecycle, session ids, and transport failures
  - desktop history/resume behavior for Gemini runs
  - parity checks against Codex/Claude event categories

## 9. Risks

- `R-1` The old placeholder note says Gemini future may be `gemini -p --output-format stream-json`, but the current codebase and SD-12 already lean on ACP. Mixing both directions without a validation gate will create drift.
- `R-2` Gemini ACP may support prompt streaming but not enough structured approval/tool/file events for full controlled-mode parity.
- `R-3` Extracting Gemini code from `sessions.go` can accidentally regress the legacy workflow session path if transport helpers are moved carelessly.
- `R-4` Resume semantics may differ between same-process ACP reuse and cross-process continuation; the adapter must not claim stronger resume guarantees than Gemini actually gives.
- `R-5` Desktop UX may assume Codex/Claude-level capabilities unless the registry and capability model stay strict.
- `R-6` Gemini may persist approval state in its own config/home. The adapter must override or clear this state per turn so FlowPilot's YOLO toggle remains authoritative.
- `R-7` Gemini MCP/tool registration may use schemas or names that conflict with FlowPilot tools. Tool readiness must be verified before the prompt starts.
- `R-8` Gemini session files may not contain enough structured transcript data for handoff-source parity. Handoff source support must stay disabled until an extractor proves otherwise.
- `R-9` Cross-account Gemini resume may be unsafe if session artifacts are account-bound. The adapter must detect this instead of treating all Gemini homes as interchangeable.

## 10. Definition of Done

- A new Gemini adapter exists in the runner and implements `ProviderRuntimeAdapter`.
- The live runner registry can return a real Gemini adapter instead of a placeholder.
- Gemini desktop chat runs through the same `/client/workflow-runs/.../turns` flow as Codex and Claude.
- Gemini emits normalized provider events into the shared event stream; no Gemini-only desktop transport is introduced.
- Gemini session id persistence and resume behavior are documented and covered by tests.
- Capability reporting is accurate for streaming, resume, approval, file events, skill selection, MCP, interrupt, and vision.
- Legacy Gemini ACP code in `sessions.go` is either extracted safely or wrapped without duplication.
- YOLO=false approval, YOLO=true auto-approval policy, and `ask_user` question UI work through the runner-owned gate.
- Gemini can invoke `spawn_agent` with blocking and non-blocking child agents through the same bridge used by Codex/Claude.
- Selected skills and context injection use the exact shared prompt assembly path and pass multi-skill tests.
- Gemini runs work in normal chat, task mode, bugfix mode, and workflow-step mode.
- `r-ca`, `r-bug`, `r-task`, and flow-gate repair prompts run correctly after Gemini turns.
- Manual and idle summary generation work for Gemini chats.
- Gemini provider accounts are deterministic, account switching is explicit, and cross-Gemini-account behavior is proven or safely blocked with typed recovery behavior.
- Gemini-as-source handoff is enabled only after a transcript extractor passes tests; Gemini-as-target behavior remains intact.
- Gemini usage/quota, account metadata, external MCP, Drive sync/restore, token/context, model metadata, replay-state, child-agent lifecycle, and attachment-fallback behavior are explicitly validated or marked unsupported with false capability flags.
- The validation pass proves a real Gemini desktop chat turn, approval-gated action, spawned child agent, generated summary, flow-rule gate, and resume path end to end.

### 10.1 DOD Verification Checklist

| DOD Item | Required Verification |
| --- | --- |
| Gemini adapter implements `ProviderRuntimeAdapter`. | Unit test constructs the adapter, asserts `Key()==ProviderKeyGemini`, verifies `Capabilities()` is explicit, and calls `SendTurn` against a fake ACP transport. |
| Live registry returns real Gemini adapter. | Registry test proves default registry remains placeholder-safe and live registry resolves Gemini only when controlled adapter dependencies are available. |
| Desktop Gemini chat uses shared turn endpoint. | Manual run starts Gemini from desktop and confirms the network path is `/client/workflow-runs/.../turns`, not a Gemini-only endpoint. |
| Normalized events only. | Event-mapper tests prove Gemini raw ACP messages become shared `ProviderEvent` types for start, delta, message complete, tool/approval/question where supported, fail, and complete. |
| Session persistence and resume. | Automated restart/resume test persists Gemini provider session id, restarts runner, resumes the chat, and verifies history continuity. |
| Capability reporting is truthful. | Capability test asserts every true flag has a passing behavior test; unsupported approval/file/vision/tool capabilities remain false until proven. |
| Legacy Gemini ACP path is safe. | Regression test covers the old `gemini_acp` session flow or confirms it is replaced by the extracted helper without changing request/response behavior. |
| Existing Codex/Claude behavior is unchanged. | Existing Codex and Claude adapter tests remain green; any shared/base file touched for Gemini has regression coverage proving previous Codex/Claude behavior still works. |
| YOLO=false approval gate. | Live or fake-transport test requests a dangerous action, receives `permission_required`, denies it, and verifies the action is not executed. |
| YOLO=true runner policy. | Test proves eligible actions are auto-approved only through runner policy and stale Gemini allowlists cannot bypass FlowPilot's YOLO toggle. |
| `ask_user` question UI. | Integration test makes Gemini call `ask_user`, verifies `user_question_required`, submits desktop answer, and confirms the turn continues. |
| `spawn_agent` parity. | Integration test makes Gemini spawn child agents with `wait=true` and `wait=false`, then verifies child run state, parent continuation, and agent panel history. |
| Skill injection exactness. | Prompt assembly test compares Gemini selected-skill prompt content/order with Codex/Claude behavior for one skill and multiple skills. |
| Context injection exactness. | Prompt assembly test verifies feature history, chat summary, and handoff context are included before the model turn. |
| Normal/task/bug/workflow modes. | Mode tests start Gemini runs in `normal_chat`, task, bugfix, and `workflow_step_auto`, then verify provider, metadata, and finalizer behavior. |
| Flow rules `r-ca`, `r-bug`, `r-task`. | Gate tests run Gemini turns that trigger each rule, verify violation events and repair prompts, and confirm repair prompts stay on Gemini. |
| Summary generation. | Manual and automated summary checks verify "Gen summary" and idle summary work for Gemini chats and inject into the next turn. |
| Account discovery and deterministic IDs. | Account tests cover default `~/.gemini`, explicit `GEMINI_HOME`, and managed `~/.geminiHomeN` with stable account IDs. |
| Cross-Gemini-account behavior. | Integration test switches between two Gemini homes and verifies safe resume under the right account or typed mismatch/recovery under the wrong account. |
| Gemini handoff target/source. | Handoff target test remains green; Gemini-as-source is enabled only after transcript extractor tests reconstruct a Gemini chat. |
| Gemini usage/quota errors. | Tests or live checks prove quota/rate-limit/auth failures produce actionable terminal provider errors and do not leak misleading login guidance or retry loops. |
| Gemini account metadata. | Account-panel tests prove all Gemini model/usage buckets render, stale metadata does not block account reset, and missing auth degrades without crashing. |
| Gemini external MCP. | Live or fake-transport check proves configured FlowPilot MCP servers are visible to Gemini with `ask_user`/`spawn_agent`, or MCP capability remains false with diagnostics. |
| Gemini child-agent lifecycle. | Matrix check covers child graph updates, approval/question gates, restart visibility, wait semantics, inherited YOLO, final result delivery, and provider-independent spawn prompt composition. |
| Gemini Drive sync/restore. | Sync/restore tests prove stale provider-account recovery from exact same-provider session evidence, or a typed unsupported state when Gemini session evidence cannot be validated. |
| Gemini token/context reporting. | Capability test asserts token/context fields are populated only from reliable ACP data and remain unavailable otherwise. |
| Gemini model/reasoning metadata. | Settings/routing tests prove supported models, display names, aliases, and reasoning controls resolve correctly or are disabled explicitly. |
| Gemini history replay state. | Replay tests reopen Gemini chats with tools, approvals, questions, child runs, and multi-turn history without duplicate rows or reopened resolved gates. |
| Gemini attachment fallback. | Vision-disabled Gemini rejects or blocks image attachments before prompt loss; future vision enablement must pass the Task-052 payload and timeline contract. |
| End-to-end parity pass. | Manual validation script records one real Gemini desktop run covering chat, approval, spawned agent, summary, flow gate, resume, and account selection. |

### 10.2 Current Verification Status

| Area | Status | Evidence |
| --- | --- | --- |
| Adapter, registry, streaming, skill injection | Implemented and unit-tested | `Task-165`; `go test ./internal/runner -run 'TestGemini|TestProviderKeyFromModel' -count=1` |
| ACP tool events, permission request mapping, MCP payload | Protocol support implemented, capability flags remain false | `Task-166`; authenticated live tool/MCP parity not yet proven |
| Session id persistence and ACP resume request | Implemented and unit-tested | `Task-167`; tests cover real session-id capture, provider-session upsert, and `session/load` request shape |
| Live authenticated Gemini run | Blocked | Local Gemini CLI `0.40.1` responds to ACP `initialize`, but `session/new` fails under the current Codex HOME because Gemini API key/config is missing; a probe with `HOME=/Users/tiendat` reaches config state but `session/new` fails with `UNSUPPORTED_CLIENT` for Gemini Code Assist for individuals |
| Gemini-as-source handoff | Disabled | No readable Gemini transcript extractor is enabled; source handoff remains blocked until live artifacts are captured |
| Full CP-40 DOD parity | Not complete | Approval, MCP, file events, resume, spawned agents, summaries, flow gates, cross-account behavior, and desktop E2E still require a supported Gemini CLI auth method accessible to the runner process before capability flags can be expanded |
