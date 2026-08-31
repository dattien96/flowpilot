# FlowPilot - AI Provider & Model Specification

While the workflow orchestrator manages tools, steps, and artifacts, the core "brain" of FlowPilot relies on the underlying Large Language Models (LLMs). This document defines how AI Providers and Models are installed, supported, and configured within the system.

## 1. AI Provider Support

The system natively supports the three primary AI coding providers:
- **GEMINI**
- **CLAUDE**
- **CODEX**

Each provider offers different strengths regarding reasoning, context windows, and execution speed.

### 1.1 Provider Installation & Detection
Because different providers may require different underlying CLI tools or SDKs, the system handles the environment setup:
- **Guided Installation:** Based on the user's OS (Mac, Windows, or Linux), when a user selects a provider, the system will execute a **Cobra Golang CLI tool** to automatically initiate the installation process for that provider.
- **Auto-Detection:** Before installation, the system checks if the required tools are already installed on the host machine. If they are already installed, the installation process is safely bypassed/ignored.
- **Provider Listing:** The system provides a command/UI to list all currently installed and supported providers (Note: the current codebase already handles part of this detection).

### 1.2 Provider Assignment per Workflow & Step
Provider is not a free-form user setting. The user selects a model, and FlowPilot derives the provider automatically from that model.
- **Workflow Level:** A workflow can store a default model (`workflows.model_override`) and YOLO mode (`workflows.yolo_mode`), and the provider is derived from that model for persistence and execution.
- **Built-in Workflows:** For built-in workflows, the user can edit the model override (`workflows.model_override`), reasoning effort override (`workflows.reasoning_effort_override`), and YOLO mode (`workflows.yolo_mode`) without cloning the workflow. All other fields (e.g. name, description, structure) remain strictly read-only.
- **Step Level:** Each step *type* (`step_definitions.model`) has its own configured model, and the provider is derived from that model.
  *Example:* A user can configure a planning step type to use `gpt-5.4`, which automatically maps to **Codex**, or `gemini-*`, which automatically maps to **Gemini**.
- **BUG-164/BUG-165 note:** step-level configuration is not a per-workflow-instance override anymore — `workflow_steps` carries no `provider_override`/`model_override`/`reasoning_effort_override` of its own. A step's model/provider is entirely the step *type's* catalog value (`step_definitions.model`), shared by every workflow that uses that step type.

---

## 2. Model Configuration

Just as providers vary, the specific model versions under those providers must be highly configurable by the user.

### 2.1 Model Selection
Each installed provider exposes multiple models (e.g., `gpt-5.4`, `gpt-5.5`, `claude-sonnet`, `gemini-pro`). The user must be able to select the exact version they want to use. If no model is configured for a workflow or step and the project default is unset, there is no silent default fallback; instead, the run is blocked and the flow/step is marked as non-runnable.

### 2.2 Model Assignment per Workflow & Step
Model configuration is the primary user-facing AI setting:
- **Workflow Level:** A user can define a single model for the entire workflow (e.g., "Use `gpt-5.4` for the whole flow"), stored on `workflows.model_override`.
- **Step Level:** Each step type has its own configured model (`step_definitions.model`) to balance intelligence and cost/speed across a workflow — e.g., a planning/architecture step type configured to run on the highly capable `gpt-5.5`, while a coding step type is configured for the faster `gpt-5.4`. This is edited on the step type's catalog entry (`Settings > Workflows > Step Definitions`), not per workflow instance (**BUG-164**: `workflow_steps` has no override columns of its own).

---

## 3. Configuration UI & Resolution Rules

To support this granular flexibility without confusing the user, the configuration resolves in a specific priority order — the *lowest* (most specific) layer that has a value wins:

1. **Step:** the step *type's* own configured model (`step_definitions.model`). Wins whenever set.
2. **Flow:** the workflow's own model (`workflows.model_override`). Used only when the step type has no configured model.
3. **Project:** the project's baseline model (`projects.default_model`). Used only when neither Step nor Flow has one.
4. **Unresolved:** If none of the above are configured, there is no hard-coded default/fallback floor (such as `gpt-5.4`). Instead, the workflow or step is unresolved and marked as non-runnable (rendered as disabled/greyed with a tooltip in the UI).

The Flow and Project tiers only apply to a **normal workflow launch** (a run started from a workflow's entry step). A **direct single-step launch** (`launchMode === "step"`, no workflow context) has no Flow tier to fall back to and does **not** fall back to Project either — it resolves **Step only**: if that step type has no configured model, the run is unresolved/non-runnable, the same as case 4 above (**BUG-229**).

Provider is derived from whichever model wins, never chosen independently — see §1.2.

**BUG-165 implementation note:** for a normal (`normal_chat`) direct-chat run, the model the user explicitly selects in the chat controller is always used as-is — this resolution order does not apply to chat mode at all, only to workflow/step-mode runs (which have no chat-controller model picker to source a choice from). For a workflow/step-mode run, this order is resolved **once, at run start** (from the workflow's entry step) to give the *run itself* one baseline model/provider.

**BUG-228 — per-node override within a running flow:** within that same run, an individual flow-graph node can still run on its own model/provider, independent of the run's baseline, depending on the node's kind:
- **Agent node** (`run: delegate`, `behavior: agent.delegate` — spawns a separate child agent run, e.g. `coder`, `reviewer_correctness`): resolves its own model from the purpose-named `step_definitions` row for its agent role (e.g. `agents/reviewer.md` → `flow-agent-delegate-reviewer`, the same "Flow: Reviewer" entry the manual workflow builder's step-type dropdown offers — **BUG-161**). Every node sharing one agent role (e.g. both reviewer nodes in a cohort) shares that role's one configured model — this is a per-role override, not a per-graph-node one. A role with no such row, or no model configured on it, falls back to inheriting the run's baseline model unchanged (pre-existing behavior).
- **Inline node** (`run: inline`, `behavior: hub.inline` — executes as the parent run's own turn, no child spawn): always uses the flow's own baseline model; there is no per-node override for inline nodes.

### 3.1 Chat Continuity SSOT — Cross-Provider Switch (CP-59 / SD-26)

> **Amendment Task-314 DOD-12 (CP-59).** This section layers the chat-level continuity contract on top of the run-level provider pinning above; it does not change `workflow`/`step` semantics.

- **Chat is the continuity unit; run is the execution unit.** A `normal_chat` conversation is identified by `chatId` (`cht_<12-hex>`, minted on first `normal_chat` `createRun` — `SD-26 §5.1`). One chat = N ordered **legs** (each leg = one real run, `runKind="chat"`, `legSeq` 0..N-1). `rs.providerKey` stays **pinned per leg** (`Task-078 T-1` preserved); switching provider always mints a new leg — no live session is migrated (`SD-26 D-1`).
- **Chat-kind gate.** Only `runKind=="chat"` legs can switch. Workflow/flow/step runs keep the `/provider` block and the single-provider lifetime verified by `Task-078`.
- **Switch is a runner-side three-phase operation** (`SD-26 S-2`): `POST /client/chats/{chatId}/switch-provider {targetProviderKey, model?, reasoningEffort?, yoloMode?}` validates guards, builds a **chat-scoped** handoff envelope (all prior legs, `SD-26 D-9`), seeds the new leg server-side, closes the old leg (`legClosedReason=provider_switch`), and appends one `chat_provider_switch` timeline record (`SD-26 E-9`). Typed refusals: `chat_not_found` 404, `chat_no_active_leg` 409 (detached), `handoff_run_busy` 409 (turn/approval/question/in-flight), `handoff_same_provider` 409 (clients fall back to in-place `session/set_config_option` / `session/set_model` per `BUG-329`), `provider_unavailable` 422 + install hint.
- **Display SSOT.** FlowPilot's durable chat transcript (`workflow_chat_events` + local NDJSON, `SD-26 D-1..D-3`) is the truth for rendering, replay, restore, and history; provider session folders (`ses_*`, claude jsonl, codex rollout, grok chat_history) demote to per-leg engine state. Timeline reads via `GET /client/chats/{chatId}/timeline` (same endpoint for TUI and Desktop — `SD-26 D-7`).
- **BUG-330 closed.** A bare-model posture pin (`plan={grok-4.5}`) derives its provider once via `providerKeyFromModel` (`provider_registry.go:202`) and persists it; cross-provider Tab/ `/mode` / `/model` / `/provider` / Desktop chips all route through the switch — a foreign model never enters the old adapter. Same-provider model changes stay in place (no new leg).
- **Restore reattach.** A Drive-restored chat has **no locally-active leg** (all legs `closed(restored)` — `SD-26 D-8`). The next user turn / Tab / provider pick reattaches via direct `createRun(chatId, switchFromRunID=latestLeg)` (not the switch endpoint) — envelope seeds the new leg identically; missing sidecars → `session_unavailable`, missing provider binary → `provider_unavailable`, chat always opens with full text. (Dev branch: always ON — no flag gate.)

---

## 4. Reasoning Effort Configuration

To optimize cost, speed, and capability for complex reasoning tasks (e.g., code reviews, spec writing), the system supports configuring **Reasoning Effort** levels alongside the model.

### 4.1 Supported Levels
- **`low`**: Optimized for speed and low cost; suitable for simple formatting or lookup steps.
- **`medium`**: Balances speed and reasoning depth for standard tasks.
- **`high`**: High-depth reasoning for complex design, auditing, or code generation.
- **`xhigh`**: Maximum reasoning depth for extremely complex, large-scale problems.

### 4.2 Resolution Hierarchy
Reasoning effort configuration follows the identical hierarchy as provider/model selection:
1. **Step Override:** Step-level override in the workflow template.
2. **Workflow/Run Override:** Specific override supplied when triggering a run.
3. **Project Default:** Project-wide baseline.

The selected reasoning effort is stored and resolved beside model selection at the project, workflow, step, and workflow-run levels. It must remain independent from provider/model choice so users can tune depth without changing the underlying model.
If no reasoning effort is selected at any level, FlowPilot defaults to `medium`.

### 4.3 Provider Execution Mapping
- **`CODEX`**: pass `-c reasoning_effort=<level>` to `codex exec`.
- **`CLAUDE`**: pass `--effort <level>` to `claude`, mapping `xhigh` to the provider's highest supported effort value.
- **`GEMINI`**: ignore the value unless the provider exposes a supported reasoning flag in a future release.
- Reasoning effort must never change provider or model selection by itself.
