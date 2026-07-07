# CP-42: Flow Pack And Generic Node Behavior Refactor

## Metadata

- Document ID: `CP-42`
- Title: `Flow Pack And Generic Node Behavior Refactor`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-01`
- Last Updated: `2026-07-01`
- Parent Documents: [CP-36: Generic Agent-Flow Engine And Review Loop](../done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [CP-41: RAG Harness Flow Mode](../done/CP-41-RAG-Harness-Flow-Mode.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Child Documents: `TBD`
- Related Documents: [CP-19: Multiple Agents](../done/CP-19-Multiple-Agents.md), [CP-35: Context And Regression Engine Rollout](../done/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-37: Prompt Context Continuity](../done/CP-37-Prompt-Context-Continuity.md)
- Replaces: `None`
- Tags: `agent-flow-engine, flow-pack, agentpack, node-behavior, generic-flow, context-package, review-loop`

## AI Quick View

### Summary

- CP-36/CP-41 introduced a generic flow vocabulary, but the implementation still contains domain-specific runner knowledge such as `plan`, `coding`, review statuses, built-in agent prompts, and CP-41 context handoff rules.
- This plan moves template-specific meaning into installable **flow packs** while keeping guarantees in Go runtime primitives.
- Agent markdown remains useful for persona and task guidance, but state transitions, schema validation, joins, caps, context production, command validation, and audit writes remain Go-enforced.
- Flow Mode becomes definition-driven: the runner sees node behaviors, inputs, outputs, edges, and policies, not hardcoded step names.
- Example pack data is added under [agentpack flow-pack](../../../apps/local-runner/internal/agentpack/flow-pack/README.md) as the target shape for future implementation.

### Current Ask

- Record the hardcode issues found in CP-36 and CP-41, define the target refactor, and provide concrete internal pack examples that show how built-in agents, flows, tool faces, contexts, and prompt templates should be represented as data.

### Key Decisions

- `P-1` Go keeps runtime guarantees; markdown and pack files configure intent, not correctness.
- `P-2` Built-in agents move from Go literals into `internal/agentpack`, with project/provider/user overrides preserving the existing precedence model.
- `P-3` Flow Mode removes step-name checks such as `isPlanStepType` and `isCodingStepType`; behavior is selected by node definition.
- `P-4` Context packages become typed artifacts produced by behavior handlers, not a CP-41-only special case.
- `P-5` Declared tool faces become pack data that map domain statuses to generic `flow_control` statuses.
- `P-6` Settings UI edits flow definitions and pack-derived nodes without requiring runner code changes for new user flows.
- `P-7` Built-in flows are read-only templates in the UI; user-created flows are separate editable definitions.
- `P-8` Built-in YAML flows must be mirrored into the definition store before execution so Chat Mode and Flow Mode both run the same normalized definition shape.
- `P-9` Chat Mode always applies the RAG/context harness baseline; optional orchestration templates such as Review Loop are selected from a built-in picker under the Chat sub-mode selector, and Review Loop is shown only when `sub_mode=bug`.

### Constraints

- Do not move correctness guarantees into `agent.md`; AI instructions are advisory and must not be trusted for state transitions.
- Preserve CP-36 primitives: `FlowNode`, `FlowEdge`, `FlowPolicy`, join, route, cap, `flow_control`, local persistence, and provider-neutral events.
- Preserve CP-41 deterministic context behavior as one built-in context producer, while allowing additional typed context producers later.
- Keep normal chat and existing agent spawn behavior compatible during rollout.
- Pack loading must validate schemas before execution; invalid pack data must fail before starting a run.

### Open Questions

- `Q-1` Which pack file format should be canonical long-term: YAML, JSON, or a typed SQLite/Supabase representation emitted by Settings UI?
- `Q-2` Should third-party context producers be compiled Go plugins, internal registered handlers only, or external MCP-style tools with strict schemas?
- `Q-3` Should built-in pack install happen once per workspace, lazily on first use, or versioned globally like `skillpack`?

### Source Refs

- CP-36 `P-1`, `P-3`, `P-6`, `DOD-1`, `DOD-7`, `R-4`.
- CP-41 `P-1` through `P-7`, especially the Plan-to-Coding handoff and Testing back-edge sections.
- Current code anchors: [flow_context_handoff.go](../../../apps/local-runner/internal/runner/flow_context_handoff.go), [flow_context_package.go](../../../apps/local-runner/internal/runner/flow_context_package.go), [agent_catalog.go](../../../apps/local-runner/internal/runner/agent_catalog.go), [agent_orchestrator.go](../../../apps/local-runner/internal/runner/agent_orchestrator.go).

## 1. Goal

Refactor the CP-36/CP-41 flow implementation so the runner remains generic while built-in templates are delivered as data packs.

The desired state:

- Chat Mode always applies the RAG/context harness baseline, matching current behavior, but optional orchestration templates such as Review Loop live in `internal/agentpack` rather than Go prompt literals.
- Chat Mode already has a sub-mode selector (`normal`, `bug`, `task`). Review Loop should attach to the Bug sub-mode only because its review-until-clean loop is most appropriate for bug-fix work.
- Flow Mode is fully definition-driven. A user can add a new flow, step, edge, policy, agent, prompt template, and typed context declaration through Settings UI without editing runner code.
- Built-in flows are selectable but read-only in Settings UI; users can clone them into editable user-owned flows.
- Runtime flow selection is explicit for optional orchestration: a chat request or UI action passes a `flowRef` for the selected built-in template plus the current `sub_mode`, while the RAG/context harness baseline is applied automatically by Chat Mode.
- Go runner enforces contracts: behavior dispatch, schema validation, state transition, parent/child routing, joins, caps, context artifact shape, command execution results, and audit write gates.
- Markdown remains replaceable guidance for agents, never the source of truth for state correctness.

## 2. Input Documents

- [CP-36: Generic Agent-Flow Engine And Review Loop](../done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md)
- [CP-41: RAG Harness Flow Mode](../done/CP-41-RAG-Harness-Flow-Mode.md)
- [CP-36 Diagrams](../done/CP-36-DIAGRAM.md)
- [CP-41 Diagrams](../done/CP-41-DIAGRAM.md)
- [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md)
- [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)

## 3. Implementation Strategy

- overall approach:
  - Keep Go as the enforcement layer.
  - Move template-specific domain text and declarations into `internal/agentpack`.
  - Introduce a node behavior registry where behavior IDs map to Go handlers with typed inputs and outputs.
  - Treat context packages as typed artifacts, not prompt-only blobs.
  - Compile Settings UI flow definitions to the same pack schema used by built-ins.
  - Resolve every optional orchestration run through a single `FlowDefinitionResolver`: built-in pack reference, mirrored Supabase definition, or user-owned Supabase definition all normalize to one runtime shape.
  - Treat Chat Mode RAG/context harness as a default baseline pack behavior, not a user-selected flow template.
- sequencing logic:
  - First inventory hardcoded CP-36/CP-41 behavior and freeze the target pack schema.
  - Then move built-in agents/prompts/tool faces into pack files while preserving current behavior.
  - Then add built-in mirror sync so pack YAML has a matching read-only definition in Supabase.
  - Then replace step-name checks with behavior declarations.
  - Finally expose built-in read-only flows and user-owned editable flows in Settings UI.
- dependencies:
  - CP-36 executor and local persistence remain the runtime foundation.
  - CP-41 context package builder remains one registered context producer.
  - Existing agent discovery and skillpack install patterns inform agentpack installation.

## 4. Work Breakdown

- `P-1` Audit current hardcodes and classify them.
  - CP-36 hardcodes to track:
    - built-in `coder`, `reviewer`, `tester`, `synthesizer` prompts in `agent_catalog.go`;
    - review-specific tool name and status schema around `submit_review_outcome`;
    - `autoReinvokePrompt` text that assumes review synthesis;
    - review loop skill/protocol living partly in Go registrations.
  - CP-41 hardcodes to track:
    - `isPlanStepType` and `isCodingStepType`;
    - `flowContextHandoffPrefix`;
    - `ComposeFlowCodingPrompt`;
    - `planContextPackage` cache field tied to a Plan/Coding pair;
    - `BuildFlowContextPackage` being implicitly associated with a step called Plan.

- `P-2` Define the pack schema.
  - `agents/*.md`: provider-agnostic agent definitions with frontmatter matching the current `AgentDefinition` parser.
  - `flows/*.yaml`: flow topology, nodes, behaviors, inputs, outputs, joins, edges, and policies.
  - `tools/*.yaml`: declared faces that expose domain-friendly tool schemas and map results to generic `flow_control`.
  - `contexts/*.yaml`: typed context artifact declarations, producer IDs, source lists, schema versions, and render templates.
  - `prompts/*.md`: render-only templates for handoff text and hub reinvocation text.
  - Reference examples live under [internal/agentpack/flow-pack](../../../apps/local-runner/internal/agentpack/flow-pack/README.md).

- `P-3` Add flow selection and definition resolution.
  - Chat Mode applies the RAG/context harness baseline automatically.
  - Chat Mode starts an optional orchestration template only when the user picks one from the built-in picker for the active sub-mode, passing a `flowRef` such as `flowpilot-core-flow-pack/review-loop`.
  - Review Loop picker is visible only when Chat `sub_mode=bug`; `normal` and `task` default to no orchestration picker unless another pack declares support for those sub-modes.
  - Flow Mode starts a user-owned flow by passing the persisted `workflow_id`/`flow_id`.
  - The resolver returns the same runtime shape for both:
    - `source=builtin_pack` for direct pack use during bootstrap;
    - `source=supabase_builtin_mirror` for read-only mirrored built-ins;
    - `source=supabase_user_definition` for user-created editable flows.
  - The runner never decides "which optional orchestration flow the user meant" by reading agent prose; selection is explicit through UI action, command/skill invocation, or a resolved `flowRef`.

- `P-4` Add built-in mirror sync.
  - On app startup or pack version change, scan `internal/agentpack/**/manifest.yaml`.
  - For each built-in flow, check whether the Supabase definition store has a matching mirror row by `packId`, `flowId`, and `version`.
  - If missing or stale, run a sync task that pushes the YAML topology, nodes, behaviors, tool faces, context declarations, and prompt refs into read-only definition rows.
  - Mark mirrored rows as `source=builtin`, `editable=false`, `pack_id`, `pack_version`, `pack_flow_id`.
  - UI shows mirrored built-ins under "Built-in flows"; users may select or clone, but not edit in place.
  - Chat Mode UI includes a built-in orchestration select below the Chat sub-mode selector.
  - The select is rendered only when at least one built-in flow declares support for the active sub-mode.
  - For `sub_mode=bug`, options include `None` and `Review Loop`.
  - `None` means plain bug chat plus the RAG/context baseline; `Review Loop` means bug chat plus the selected orchestration template.

- `P-5` Add a node behavior registry in Go.
  - Minimum behavior IDs:
    - `agent.delegate`
    - `hub.inline`
    - `context.produce`
    - `context.render`
    - `command.validate`
    - `validation.summarize`
    - `artifact.audit_draft`
    - `flow.control`
    - `user.confirm`
  - Each handler declares input schema, output schema, and whether it can mutate workspace state.
  - The executor dispatches by behavior ID; it does not branch on semantic step names.

- `P-6` Move built-in agents from Go literals into `internal/agentpack`.
  - Add install/discovery code parallel to `internal/skillpack`.
  - Built-in agents remain low-precedence defaults and are file-overridable.
  - Remove prompt literals from `builtinAgentDefinitions` once pack loading is live.
  - Preserve existing project-local and provider-home precedence.

- `P-7` Convert Review Loop to a pack.
  - Use [review-loop.yaml](../../../apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml) as the target shape.
  - Use [submit-review-outcome.yaml](../../../apps/local-runner/internal/agentpack/flow-pack/tools/submit-review-outcome.yaml) as the declared-face example.
  - Go validates tool input and maps declared statuses to `continue|done|escalate`.
  - Reviewer/synthesizer markdown may change, but cannot directly transition state.
  - Chat Mode can select this flow through the built-in picker only when `sub_mode=bug` and send its `flowRef`; Flow Mode can select its mirrored read-only definition or a cloned editable copy.

- `P-8` Convert CP-41 RAG Harness to a pack.
  - Use [rag-harness.yaml](../../../apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml) as the target shape.
  - Replace `isPlanStepType`/`isCodingStepType` with declared behaviors:
    - context producer node emits `flow_context_package.v1`;
    - coding node consumes that context artifact through a prompt template;
    - validation node emits validation result or retry context;
    - audit node emits a draft artifact behind confirmation.
  - Existing `BuildFlowContextPackage` becomes handler `deterministic_feature_context`.
  - Flow Mode normally runs this through the mirrored definition store, not by special-casing the YAML file at runtime.
  - Chat Mode uses this pack's context behavior as an auto-on baseline, not as a user-selected orchestration flow.

- `P-9` Add Settings UI support for pack-backed flow authoring.
  - Built-in flows are listed in a read-only "Built-in flows" section.
  - Chat Mode has a compact "Built-in orchestration" select below the sub-mode selector.
  - The select is sub-mode aware: Review Loop appears for Bug mode, not for Normal or Task mode.
  - RAG/context harness is shown as enabled baseline behavior for Chat Mode, not as an editable picker option.
  - Built-in rows expose `Run`, `Clone`, and `View definition`, but no direct edit action.
  - Cloning creates a user-owned editable flow definition with copied nodes/edges/policies.
  - Users choose node behavior from registered behavior IDs.
  - Users choose agent files for delegate nodes.
  - Users choose typed context producers and prompt templates.
  - UI validates edges, joins, status mappings, and policy caps before saving.
  - Saved definitions compile to the same schema used by built-in packs.

- `P-10` Deprecate step-name semantics.
  - Remove `isPlanStepType` and `isCodingStepType` once equivalent behavior declarations exist.
  - Add guard tests proving the executor does not contain domain names such as `plan`, `coding`, `reviewer`, `approved`, or `changes_requested`.
  - Leave backward-compatible migration code that maps legacy step types to pack behavior declarations during import only.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/agent_catalog.go`
  - `apps/local-runner/internal/runner/flow_context_handoff.go`
  - `apps/local-runner/internal/runner/flow_context_package.go`
  - `apps/local-runner/internal/runner/agent_orchestrator.go`
  - `apps/local-runner/internal/runner/provider_registry.go`
  - `apps/local-runner/internal/runner/codex_adapter.go`
  - `apps/local-runner/internal/runner/claude_mcp_server.go`
  - `apps/local-runner/internal/runner/claude_permission_mcp.go`
  - `apps/local-runner/internal/skillpack/*`
  - `apps/local-runner/internal/agentpack/*`
  - desktop Settings UI flow editor files under `apps/desktop`
- modules:
  - local-runner flow executor
  - agent catalog and built-in install path
  - declared tool face registration
  - context artifact production and rendering
  - workflow/flow Settings UI
- database:
  - no new run-state table required.
  - existing definition tables may need fields for behavior ID, input/output artifact bindings, prompt template path, declared tool face references, pack source, pack version, and read-only/editable flags.
- external systems:
  - no new provider API requirement.
  - no vector DB introduced by this plan.

## 6. Data or Migration Steps

- schema:
  - Add or map definition fields for `behaviorId`, `inputs`, `outputs`, `promptTemplate`, `toolFaces`, and `contextArtifacts`.
  - Add or map definition fields for `source`, `editable`, `packId`, `packVersion`, and `packFlowId` for built-in mirrors.
  - Keep run state in local session persistence per CP-36.
- data backfill:
  - Add an idempotent built-in mirror sync task: YAML pack -> Supabase definition rows.
  - The sync task must run when a built-in flow is selected and its mirror is missing, and may also run on app startup.
  - Migrate existing workflow steps with `step_type in {plan, planning, design}` to `behaviorId=context.produce` when importing CP-41-style flows.
  - Migrate existing workflow steps with `step_type in {coding, implementation, code}` to `behaviorId=agent.delegate` plus input binding when importing CP-41-style flows.
  - Migrate built-in Go agents into `internal/agentpack/flow-pack/agents`.
- config updates:
  - Add pack manifest and version metadata.
  - Add pack install/update behavior parallel to skillpack.
  - Add compatibility warnings for legacy flow definitions that rely on step-name semantics.

## 7. Validation Plan

- tests to add:
  - pack parser validates agents, flows, tools, contexts, and prompts.
  - built-in mirror sync is idempotent and updates stale mirrors only when pack version changes.
  - starting Chat Mode with no orchestration selection still applies the RAG/context baseline.
  - selecting Review Loop in the Chat Mode Bug sub-mode built-in picker resolves to the mirrored definition when present and triggers mirror sync when missing.
  - flowRef start recreates a missing built-in mirror row on demand and preserves the normalized pack shape (`Definition.ID`, contexts, node inputs/outputs).
  - built-in flowRef start supports inline entry nodes and emits the expected context package / wait-note events instead of stalling.
  - switching Chat sub-mode away from Bug hides or clears the Review Loop selection.
  - after the first chat turn, intent / `flowRef` selection stays locked and cannot drift from the run's already-persisted mode.
  - selecting a user flow in Flow Mode never mutates built-in mirror rows.
  - behavior registry rejects unknown behavior IDs before run start.
  - Review Loop pack produces the same node/edge/policy behavior as current CP-36 path.
  - forward-edge auto-spawn from `edges_json` launches the reviewer cohort deterministically for flowRef-started runs.
  - RAG Harness pack produces the same prompt/context behavior as current CP-41 path.
  - changing `reviewer.md` changes reviewer instruction text but not route/cap/join behavior.
  - project-local or provider-home agent files cannot shadow pack-owned built-in flow agents during executor-driven spawns.
  - invalid declared-face status mapping is rejected before tool registration.
  - `submit_review_outcome` is only advertised to allowed hub turns and is rejected for normal chat / non-hub child turns.
  - generic-flow guard test rejects domain-specific strings in executor code.
- manual checks:
  - install built-in pack into a fresh workspace and verify normal Chat Mode uses the RAG/context baseline with no orchestration picker selection.
  - select Bug sub-mode, choose Review Loop in the built-in picker, and verify it starts the Review Loop template.
  - for the built-in Review Loop start path, verify the hub gets the wait note, does not also code itself, and the reviewer cohort is auto-spawned from the flow edges.
  - select Normal or Task sub-mode and verify Review Loop is not offered.
  - start a plain normal chat and verify `submit_review_outcome` is not offered; then start Review Loop and verify only the hub/synthesis turn can use it.
  - after the first chat turn completes, verify the intent / `flowRef` picker is locked and explains that changing it requires a new chat.
  - delete the mirrored built-in definition in a test database, select the built-in flow, and verify the sync task recreates it before run start.
  - delete the mirrored built-in RAG Harness definition and verify restart/recreate still preserves flow contexts plus node inputs/outputs, and the inline entry node executes.
  - clone a built-in flow in Settings UI, edit the clone, and verify the built-in remains read-only and unchanged.
  - edit `reviewer.md` and verify the prompt changes without code changes.
  - create a custom Flow Mode flow in Settings UI with a non-Plan/non-Coding node naming scheme and verify it runs through behavior declarations.
  - restart runner after context production and verify typed artifacts restore correctly.
- failure cases:
  - missing agent file.
  - invalid prompt template reference.
  - context producer emits malformed artifact.
  - declared tool face maps to unsupported generic status.
  - a non-hub run attempts to call `submit_review_outcome`.
  - user deletes a behavior referenced by an existing flow.
  - legacy flow definition has only `step_type` and no `behaviorId`.
  - built-in YAML exists locally but Supabase mirror is stale or missing.
  - built-in mirror row was manually edited despite `editable=false`.
  - workflow-step replacement insert fails mid-save; existing steps must remain intact instead of being deleted first.

## 8. Rollout and Fallback

- rollout order:
  - Ship pack files as inert examples.
  - Add parser and validation with no runtime behavior change.
  - Add read-only built-in mirror sync and UI listing.
  - Load built-in agents from pack while keeping Go built-ins as fallback.
  - Load Review Loop from pack behind a feature flag.
  - Load CP-41 RAG Harness from pack behind a feature flag.
  - Enable Settings UI creation/editing of pack-backed flows.
  - Remove domain-specific step-name branches after migration coverage is green.
- fallback path:
  - If pack loading fails, fall back to existing Go built-ins during the transition window.
  - If built-in mirror sync fails, block built-in flow start with a clear validation error instead of running an unknown/stale definition.
  - If a user flow references an unknown behavior, block run start and show validation errors.
  - Keep legacy chat behavior available until Review Loop pack parity is proven.
- monitoring:
  - log pack name/version, loaded behavior IDs, agent override source, declared tool faces, and context artifact schema versions.
  - emit validation errors with file path and field path.

## 9. Risks

- `R-1` Over-generic packs can become prompt-only automation. Mitigation: behaviors remain Go handlers with typed schemas.
- `R-2` User-editable agent markdown may reduce AI quality. Mitigation: markdown can affect reasoning quality but not transition correctness.
- `R-3` Behavior registry becomes a second workflow engine. Mitigation: registry only dispatches node work; CP-36 executor remains the single router.
- `R-4` Pack schema churn can break Settings UI. Mitigation: version pack manifests and run migrations through explicit schema versions.
- `R-5` Context generalization can hide CP-41 deterministic guarantees. Mitigation: keep `deterministic_feature_context` as a typed producer with tests and no vector dependency.
- `R-6` Legacy CP-41 docs mention `workflow_run_steps` more strongly than CP-36's local persistence decision. Mitigation: CP-42 treats definitions as durable data and run state as local-session data.

## 10. Definition of Done

- [x] A validated `internal/agentpack` schema exists for agents, flows, tools, contexts, and prompt templates.
- [x] Built-in flows are mirrored into definition storage as read-only rows and recreated by an idempotent sync task when missing.
- [x] Chat Mode applies the RAG/context baseline automatically and can optionally select Review Loop via explicit `flowRef` only in Bug sub-mode; Flow Mode can select either a read-only built-in mirror or a user-owned editable flow.
- [x] Built-in agents are loaded from pack files with Go literals retained only as temporary fallback.
- [ ] Review Loop runs from a pack definition with no review-specific transition branch in the executor.
- [ ] RAG Harness runs from a pack definition with no `isPlanStepType` or `isCodingStepType` runtime branch.
- [ ] Context packages are typed artifacts with producer/consumer bindings defined by flow data.
- [ ] Settings UI can create a flow using behavior IDs, agent files, context artifacts, edges, joins, and policies without runner code changes.
- [ ] Domain-free guard tests prove executor code does not contain template-specific role/status names.
- [ ] Existing CP-36/CP-41 manual flows still pass after migration.

## 11. Progress Notes

- Implemented the embedded `internal/agentpack` parser and validator for manifests, agents, flows, tools, contexts, and prompt templates.
- Loaded built-in agents from pack data through the runner catalog, with legacy Go built-ins kept as fallback.
- Moved the built-in review-loop and RAG-harness flow definitions into pack YAML and kept the runner's legacy helpers as compatibility fallback.
- Replaced `isPlanStepType` / `isCodingStepType` with behavior-ID normalization aliases so step matching now follows pack data.
- Moved the review-loop auto-reinvoke prompt and coder re-entry prompt into pack templates while keeping typed issue formatting in Go.
- Added CA notes for the implemented slices: [CA-147](../../../change-audit/CA-147-agent-flow-pack-and-generic-node-behavior-refactor.md) and [CA-148](../../../change-audit/CA-148-pack-driven-coder-reentry-prompt.md).
- Added the node behavior registry (`P-5`) with the nine core behavior IDs and Go handlers; `context.produce`/`context.render`/`flow.control` wrap the existing CP-41/CP-36 helpers rather than duplicating logic. See [CA-149](../../../change-audit/CA-149-node-behavior-registry-and-dispatch.md).
- Added `FlowDefinitionResolver`/`FlowMirrorSyncService` (`P-3`/`P-4`) with a pluggable `FlowDefinitionStore` contract, tested against an in-memory fake and a real `FileFlowDefinitionStore` (JSON files under `<workspace>/.flowpilot/flow-definitions/`) — idempotent sync, hash-change detection, clone, read-only rejection, and local-only pack fallback all pass against the real implementation. See [CA-150](../../../change-audit/CA-150-flow-definition-resolver-and-mirror-sync.md) and [CA-154](../../../change-audit/CA-154-file-backed-flow-definition-store-and-chat-contract.md).
- Wired `injectFlowContextIfCoding`'s context build/render steps through `DefaultBehaviorRegistry()` dispatch instead of calling `BuildFlowContextPackage`/`ComposeFlowCodingPrompt` directly, so the behavior registry is load-bearing on the live RAG-harness path, not just inert infrastructure. See [CA-151](../../../change-audit/CA-151-rag-harness-behavior-dispatch-rewire.md).
- Added `BuiltinOrchestrationOptions(subMode)`, a pure pack-metadata-driven computation of which built-in flows a future Chat Mode picker should offer per sub-mode, plus `subMode`/`flowRef` fields on the chat turn request contract (`turnBody`) with `handleStartTurn`-level validation rejecting any `flowRef` not in that option set (including the chat-baseline flow itself). See [CA-152](../../../change-audit/CA-152-chat-builtin-orchestration-options.md) and [CA-154](../../../change-audit/CA-154-file-backed-flow-definition-store-and-chat-contract.md).
- Added a frozen domain-hardcode inventory guard test plus migration tests proving legacy step types and unrelated custom step types both behave correctly. See [CA-153](../../../change-audit/CA-153-domain-hardcode-guard-and-migration-tests.md).
- Consolidated the four scattered `isAgentRole(rs, "coder")` call sites in `interactive_service.go` into one named, tested `isCoderRun` shim (a manual call-graph read — GitNexus still not connected — showed three of the four sites are already segregated behind the `loopMode == "explicit"` flag, bounding this to a safe extract-function refactor; verified behavior-identical against the full `TestE2EReviewLoop*` suite). Hardcode baseline for that file dropped from 4 to 1. See [CA-155](../../../change-audit/CA-155-consolidate-coder-role-check-shim.md).
- **Correction to an earlier claim in this doc:** a prior session pass claimed no `subMode`/chat-intent concept exists anywhere in `apps/desktop-flowpilot`; that was wrong — a grep for the literal string `"bug"` missed the desktop app's existing `ChatStartMode` (`"normal"|"task"|"bugfix"`) selector, whose Bug value is spelled `"bugfix"`. Two independent research passes confirmed a working 3-tab UI (`ChatStartIntentPanel`) already existed. Built the Chat Mode "Built-in orchestration" picker directly into that existing panel instead: a new read-only `GET /client/chat/builtin-orchestration-options` endpoint backed by `BuiltinOrchestrationOptions`, a Zustand `flowRef`/`builtinOrchestrationOptions` slice with T-7's clear-on-mode-change behavior, and a `<select>` in `ChatStartIntentPanel` shown only for the Bug intent. See [CA-156](../../../change-audit/CA-156-chat-builtin-orchestration-picker-ui.md).
- **A resolved `flowRef` now actually executes.** Investigated how Review Loop starts today: an AI hub agent decides to call `spawn_agent` with `autoOrchestrate: true` itself — there is no Go-side auto-start. `spawnChildRun` (the function `spawn_agent` tool calls land on) was already decoupled from HTTP/tool-parsing, so `startTurn`'s existing first-turn gate now also spawns a resolved flow's entry node (found via `dependsOn`-free `agent.delegate` nodes) through that exact same path, async and with `AutoOrchestrate: true`. Every existing cohort/join/`isCoderRun`/hub-reinvoke/`flow_control` mechanism is completely unmodified and drives the rest, exactly as it does for an AI-initiated flow. See [CA-158](../../../change-audit/CA-158-generic-flow-executor-entry-node-spawn.md).
- **User-directed architectural pivot, superseding the standalone `flow_definitions` table (CA-150/154/157/159): built-in flows now mirror into the existing `workflows`/`workflow_steps` tables, and the existing "Workflows/Steps" Settings screen (`WorkflowsSettings.tsx`) is the real authoring UI — no new screen, no parallel schema.** `workflows` gained `is_builtin`/`editable`/`cloneable`/`cloned_from`/pack-identity/policy/`edges_json`; `workflow_steps` gained `node_id`/`behavior_id`/`agent_ref`/`depends_on_json`/`join_mode`/`cohort` (nine generic `step_definitions` rows seeded so a mirrored node satisfies the existing `step_type` FK). `SupabaseWorkflowFlowStore` replaces the removed file/Supabase `flow_definitions`-table stores; the dead `GET/POST/PUT /client/flows*` local-runner endpoints were removed (Settings never called them — it talks to Supabase directly via `packages/flowpilot-client-core`). `WorkflowsSettings.tsx` now shows a "Built-in" badge, swaps Save/Delete for "Clone" on non-editable rows, gates the step editor read-only for built-ins, and exposes the new per-node fields in the step form. This closes Task-179 for real. See [CA-160](../../../change-audit/CA-160-flow-definitions-migrated-to-workflows-table.md).
- **Hub-framing question resolved by explicit user direction**: when a flowRef auto-spawns the entry node, the hub now receives a pending note (`prompts/flow-start-wait.md`, pack-driven) telling it an agent already started and to wait rather than redundantly try to also write the code itself. See CA-160.
- **The "continue" reinvoke target is now edge-driven, per explicit user direction.** Investigation narrowed the real gap: forward progression (coder→reviewers→synthesis) is already AI-decided via `spawn_agent`, not a Go hardcode — the one genuine hardcode was the back-edge (`synthesis` says `continue` → reinvoke "the coder" by role-name scan). `interactiveRun.activeFlowEdges` (set by `startResolvedFlow`) + `resolveContinueBackEdgeTarget` now resolve that target from the flow's own `synthesis->coder` back-edge, matching the spawned child by `label` — `isCoderRun` is bypassed entirely on this path and remains only as the fallback for AI-initiated runs with no tracked edges. Verified byte-identical to prior behavior on the existing `TestE2EReviewLoop*` suite. See [CA-161](../../../change-audit/CA-161-edge-driven-continue-reinvoke.md).
- **Deliberately not attempted, with reasons on record:**
  - Making the *forward* edges (coder→reviewers→synthesis) edge-driven too — this is already AI-decision-driven, not a Go-side hardcode, so there is nothing to generalize there without removing the hub's legitimate reasoning role.
  - Live browser verification of the new `WorkflowsSettings.tsx` UI — the desktop app requires real Supabase sign-in with no demo/bypass mode.
  - Applying the schema migration to a live Supabase project — no database access this session.
- Remaining CP-42 items: the items under "not attempted" above (live UI verification, applying the migration) — the hardcode-elimination work itself is now closed as far as the live loop's genuine Go-side decisions go.
