# FlowPilot Tech Design - AI Provider & Model Integration

This document translates `SS-05-Workflow-Ai-Provider` into technical implementation details.

## 1. UI/UX Design (Frontend - React)
- **Settings Screen:**
  - Show the three supported providers: `GEMINI`, `CLAUDE`, `CODEX`.
  - Show host-machine detection status per provider: `INSTALLED`, `NOT_INSTALLED`, `FAILED`, `AUTH_REQUIRED`, or `UNSUPPORTED_OS`.
  - Show detected CLI version when installed.
  - Show the exact selectable models for each provider.
  - Show and persist project-level defaults for Model and Reasoning Effort.
  - Show the derived provider as a read-only badge or label next to the selected model.
  - Expose actions to `Refresh Detection` and `Install`; show `Authenticate` only when the selected provider exposes an explicit auth flow, otherwise surface auth readiness and the next required host-side step.
- **Workflow Launch UI:**
  - When starting a workflow run, let the user override the project default Model and Reasoning Effort for that run.
  - The selected run-level values become the baseline for that `workflow_run`.
- **Workflow / Step Override UI:**
  - In the Workflow Builder, allow override of Model and Reasoning Effort at the workflow-definition level and the individual-step level.
  - The model dropdown is the source of truth; provider is derived automatically from the selected model.
  - The reasoning-effort selector must be visible alongside the model selector.
  - If a step has no override, the UI should show that it inherits from workflow/run/project defaults, with `gpt-5.4` and `medium` as the final fallbacks.
- **Validation UX:**
  - Prevent save/start when a selected model is not supported by the selected provider inventory.
  - Warn before run start when the derived provider is not installed or still requires authentication.
  - Validate that reasoning effort, when provided, is one of the supported levels.

---

## 2. Provider Listing and Detection Contract

The Go-Runner's Cobra CLI must expose a machine-readable command for the frontend:

```bash
flowpilot providers list --json
```

This command is the source of truth for "installed and supported providers". It must not rely on stale frontend cache.

### 2.1 Detection Rules
- Supported providers are hardcoded in the product: `GEMINI`, `CLAUDE`, `CODEX`.
- Installed state is detected on the host machine by using `exec.LookPath(...)` against the provider CLI binary.
- Version is resolved by invoking `<provider-binary> --version` after detection.
- Authentication state is resolved by a lightweight provider-specific readiness check when possible.
- Detection is host-level state, not project-level state. Do not persist install/auth state in `projects`.

### 2.2 Response Contract

`flowpilot providers list --json` returns:

```json
{
  "providers": [
    {
      "id": "CODEX",
      "key": "codex",
      "label": "Codex",
      "supported": true,
      "installed": true,
      "install_status": "INSTALLED",
      "auth_status": "READY",
      "detected_binary": "codex",
      "detected_version": "x.y.z",
      "version": "x.y.z",
      "binaryPath": "C:\\Users\\you\\AppData\\Local\\Programs\\codex\\codex.exe",
      "installHint": "npm install -g @openai/codex",
      "models": [
        {
          "id": "codex-5.5",
          "display_name": "codex-5.5",
          "available": true,
          "source": "registry"
        }
      ],
      "last_error": null
    }
  ]
}
```

The Admin Web provider settings page may normalize this payload into its UI model, but the runner contract above is the source of truth.

### 2.3 Status Semantics
- `supported`: FlowPilot knows how to install, detect, and execute this provider on the current app version.
- `install_status`:
  - `INSTALLED`: CLI found and version check passed.
  - `NOT_INSTALLED`: CLI not found.
  - `FAILED`: install or verification failed.
  - `UNSUPPORTED_OS`: FlowPilot does not have a valid install path for the current OS.
- `auth_status`:
  - `READY`: provider is installed and usable.
  - `AUTH_REQUIRED`: provider CLI exists but still needs login or API key setup.
  - `UNKNOWN`: FlowPilot cannot reliably determine auth state without a real execution.

---

## 3. Provider Installation Commands

The Go-Runner's Cobra CLI will implement:

```bash
flowpilot install-provider <provider_name>
```

Before installing, the runner uses `exec.LookPath` to auto-detect whether the CLI is already present. If detection succeeds, installation is skipped and verification still runs.

### 3.1 OS Selection Rules
- Resolve OS using Go runtime detection:
  - `windows` -> Windows install branch
  - `darwin` -> macOS install branch
  - `linux` -> Linux install branch
- Treat WSL as Linux for shell-based installers.
- Prefer an official native installer when the provider publishes one for the current OS.
- Use npm-based installation only for providers whose official distribution path is npm-based on that platform.
- If the current OS has no supported install path, return `UNSUPPORTED_OS` without mutating project data.

### 3.2 Claude Code (Anthropic)
- **Official docs:** https://code.claude.com/docs/en/overview
- **macOS / Linux / WSL:**
  ```bash
  curl -fsSL https://claude.ai/install.sh | bash
  ```
- **Windows (PowerShell):**
  ```powershell
  irm https://claude.ai/install.ps1 | iex
  ```
- **Windows (Command Prompt):**
  ```cmd
  curl -fsSL https://claude.ai/install.cmd -o install.cmd && install.cmd && del install.cmd
  ```
- **macOS (Homebrew alternative):**
  ```bash
  brew install --cask claude-code
  ```
- **Verification:**
  ```bash
  claude --version
  ```
- **Prerequisites:** Anthropic account with Claude Pro, Team, or Enterprise subscription, or API key via Anthropic Console.

### 3.3 Codex CLI (OpenAI)
- **All platforms (npm):**
  ```bash
  npm install -g @openai/codex
  ```
- **macOS (Homebrew alternative):**
  ```bash
  brew install codex
  ```
- **Prerequisites:** Node.js >= 18. After install, run `codex` or `codex auth` to authenticate with ChatGPT account or OpenAI API key.
- **Verification:**
  ```bash
  codex --version
  ```

### 3.4 Gemini CLI (Google)
- **All platforms (npm):**
  ```bash
  npm install -g @google/gemini-cli
  ```
- **No-install alternative:**
  ```bash
  npx @google/gemini-cli
  ```
- **Prerequisites:** Node.js >= 20. On first run, user is prompted to sign in with Google account.
- **Verification:**
  ```bash
  gemini --version
  ```

---

## 4. Model Registry and Discovery

### 4.1 Model Selection
- The system stores the exact provider-native model identifier selected by the user.
- FlowPilot does not normalize model names across providers. `codex-5.5` and `gemini-1.5-pro` are provider-specific IDs and must be stored verbatim.

### 4.2 Model Source of Truth
- Each provider returned by `flowpilot providers list --json` must include a model list.
- The runner builds that model list using this priority:
  1. Provider-specific runtime introspection, if the CLI exposes a safe model-list capability.
  2. Bundled FlowPilot registry of known supported model IDs for that provider.
  3. User-entered exact model string only as a manual fallback for advanced settings.
- If a provider CLI does not support runtime model discovery, FlowPilot still works by using the bundled registry.

### 4.3 Availability Rules
- A model is selectable only when it belongs to the currently selected provider.
- A model may be shown but marked unavailable when:
  - the provider is not authenticated yet,
  - the provider account tier does not expose that model,
  - the bundled registry knows the model but the runner cannot verify access yet.
- At execution time, the runner must re-validate that the resolved model belongs to the resolved provider before making the LLM call.

---

## 5. Provider Instruction Files

Each AI provider uses a different file to receive persistent project-level instructions:

| Provider | Instruction File | Location | Skills Location |
|----------|------------------|----------|-----------------|
| Claude   | `CLAUDE.md`      | Project root | `.claude/skills/<name>/SKILL.md` |
| Codex    | `AGENTS.md`      | Project root | `.codex/skills/<name>/SKILL.md` (or via `config.toml`) |
| Gemini   | `GEMINI.md`      | Project root | `.gemini/skills/<name>/SKILL.md` |

This is critical for the Workflow Engine. FlowPilot must generate and write to the correct file depending on which provider is selected for a step. This behavior must stay aligned with `SD-05-Workflow-Engine`, section `5. Persistent Files That DO Live in LLM Folders`.

---

## 6. Configuration Resolution Logic

### 6.1 Persistence Locations
- `projects.default_provider`, `projects.default_model` and `projects.default_reasoning_effort` store project-wide defaults.
- `workflows.provider_override`, `workflows.model_override` and `workflows.reasoning_effort_override` store workflow-definition-level data, with provider derived from model.
- `step_definitions.model` stores the step *type's* own configured model, shared by every workflow that uses that step type. **BUG-164**: `workflow_steps` has no `provider_override`/`model_override`/`reasoning_effort_override` of its own anymore — a step's model/provider is entirely its step type's catalog value, not a per-workflow-instance override.
- `workflow_runs.provider`, `workflow_runs.model` and `workflow_runs.reasoning_effort` store the resolved run-level baseline chosen when a workflow starts.
- `ai_runs.provider`, `ai_runs.model_name` and `ai_runs.reasoning_effort` store the actual parameters used for each individual model invocation.

### 6.2 Resolution Order
**BUG-165**: for a workflow/step-mode run, the Go-Runner resolves Model in this order at run start, then derives Provider from the resolved model — the lowest (most specific) layer with a value wins:
1. **Step**: the entry step's own `step_definitions.model`.
2. **Flow**: the workflow's `workflows.model_override`.
3. **Project**: `projects.default_model`.
4. **Default**: the hard-coded floor `gpt-5.4`.

Reasoning Effort has no equivalent step-type-level tier today (`step_definitions` has no `reasoning_effort` resolution role) and continues to resolve `launchOverride.reasoning_effort ?? workflow.reasoning_effort_override ?? project.default_reasoning_effort ?? "medium"`.

- **Chat mode is exempt**: a `normal_chat` (direct chat) run has no workflow/step context and no chat-controller equivalent for workflow mode — the model the user explicitly selects in the chat controller is sent and used as-is, never routed through this resolution order.
- Provider is never edited independently; it is derived from the resolved model.
- **Known limitation**: this order is resolved **once, at run start**, from the workflow's entry step. It is not re-resolved as execution advances to later steps within the same run — genuine per-step model switching during a single run's execution is not yet implemented (tracked as follow-up work, not covered by this resolution logic).

### 6.3 Runner Pseudocode

```text
// Workflow/step-mode run start (createRun) — BUG-165.
// Chat mode (normal_chat) skips this entirely and uses launchOverride.model as-is.
runModel = launchOverride.model
    ?? entryStep.step_definitions.model   // Step
    ?? workflow.model_override            // Flow
    ?? project.default_model              // Project
    ?? "gpt-5.4"                          // Default

runReasoning = launchOverride.reasoning_effort
    ?? workflow.reasoning_effort_override
    ?? project.default_reasoning_effort
    ?? "medium"

runProvider = providerFrom(runModel)
```

### 6.4 Validation Rules
- Reject run start if the resolved run-level model is not supported.
- Reject step execution if the resolved step-level model is not supported.
- Reject run start if no model/reasoning combination can be resolved after applying the fallback chain.
- Reject step execution or run start if the resolved reasoning effort level is invalid (not low, medium, high, or xhigh).

---

## 7. Persistence and Data Contract Alignment

This document must stay aligned with `SD-05-Workflow-Engine`.

### 7.1 Required Stored Fields
- `projects` must contain at least:
  - `id`
  - `default_model`
  - `default_reasoning_effort`
  - `default_provider` as a derived reference field
- `workflows` must contain:
  - `model_override`
  - `reasoning_effort_override`
  - `provider_override` as a derived reference field
- `step_definitions` must contain:
  - `model` (the step type's own configured model — **BUG-164**: `workflow_steps` itself carries no override columns)
- `workflow_runs` must contain:
  - `model`
  - `reasoning_effort`
  - `provider` as a derived reference field

### 7.2 Host-State vs Project-State Boundary
- Provider installation state, binary path, detected version, and auth readiness are host-machine concerns owned by the Go-Runner.
- They must be returned by `flowpilot providers list --json`.
- They should not be treated as project configuration fields because one machine can host many projects, and one project can be opened on many machines.

### 7.3 Step Runtime Display
- The execution dashboard may show Provider/Model/Reasoning per step without adding new step-runtime columns.
- The UI derives the displayed step Provider/Model by combining:
  - `step_definitions.model` (the step type's own configured model)
  - `workflow_runs.model` / `workflow_runs.reasoning_effort` (the run's resolved baseline, itself already Step > Flow > Project > default per §6.2)
- If later audit requirements demand immutable per-step runtime tracing, add `resolved_provider`, `resolved_model`, and `resolved_reasoning_effort` to `workflow_run_steps`.

---

## 8. Go-Runner Installation and Execution Flow

1. Frontend loads provider inventory by calling `flowpilot providers list --json`.
2. User selects project default Model/Reasoning in Settings -> persist to `projects.default_model` and `projects.default_reasoning_effort`, then derive `projects.default_provider` from the model.
3. User optionally sets workflow-definition overrides in `workflows.model_override` / `workflows.reasoning_effort_override`.
4. User configures each step *type's* own model on its `step_definitions` catalog entry (`Settings > Workflows > Step Definitions`) — not a per-workflow-instance override (**BUG-164**).
5. When starting a workflow/step-mode run, the Go-Runner resolves the model itself (Step > Flow > Project > default, §6.2) since the frontend passes no launch-time model for this mode (**BUG-165**); a `normal_chat` run instead passes the model the user explicitly selected in the chat controller.
6. The Go-Runner resolves `workflow_runs.model` and `workflow_runs.reasoning_effort` using the precedence rules in Section 6, derives `workflow_runs.provider` from the model, then persists the run.
7. Before the first LLM call, the runner checks whether the resolved provider is installed and authenticated.
8. If not installed, the runner executes the OS-appropriate install command from Section 3.
9. After install, the runner runs `<provider> --version` to verify detection, then refreshes provider inventory.
10. If authentication is still required, the runner launches the provider-specific login flow and marks the provider `AUTH_REQUIRED` until the next successful readiness check.
11. Before each step execution, the runner resolves the final step Model and Reasoning Effort, derives Provider from the model, and validates the parameters.
12. The runner writes prompts to the provider-specific instruction and skill locations, then executes the step with the resolved parameters. If reasoning effort is set:
    - For **Codex**: appends `-c reasoning_effort=<level>` to the `codex exec` call.
    - For **Claude Code**: appends `--effort <level>` to the `claude` CLI call (with `xhigh` mapped to `max`).
    - For other CLI providers: appends the provider-specific flag if supported.
