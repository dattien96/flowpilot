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
- **Workflow Level:** A workflow can store a default model, and the provider is derived from that model for persistence and execution.
- **Step Level (Granular override):** Each individual step can store its own model, and the provider is derived from that model.
  *Example:* A user can choose `gpt-5.4` for planning steps, which automatically maps to **Codex**, or choose a `gemini-*` model, which automatically maps to **Gemini**.

---

## 2. Model Configuration

Just as providers vary, the specific model versions under those providers must be highly configurable by the user.

### 2.1 Model Selection
Each installed provider exposes multiple models (e.g., `gpt-5.4`, `gpt-5.5`, `claude-sonnet`, `gemini-pro`). The user must be able to select the exact version they want to use. If the user does not choose a model, FlowPilot defaults to `gpt-5.4`, which maps to **Codex**.

### 2.2 Model Assignment per Workflow & Step
Model configuration is the primary user-facing AI setting:
- **Workflow Level:** A user can define a single model for the entire workflow (e.g., "Use `gpt-5.4` for the whole flow").
- **Step Level (Granular override):** A user can override the model on a per-step basis to balance intelligence and cost/speed.
  *Example:* A user can configure the planning and architecture steps to use the highly capable `gpt-5.5`, but configure the actual implementation/coding step to use the faster `gpt-5.4`.

---

## 3. Configuration UI & Resolution Rules

To support this granular flexibility without confusing the user, the configuration resolves in a specific priority order. We need to let users configure this effortlessly:

1. **Project Default:** The user sets a baseline Model and Reasoning Effort for the entire project. Provider is derived from the model and stored only as reference data.
2. **Workflow Default:** When triggering a workflow, the user can override the Project Default Model and Reasoning Effort for that specific run.
3. **Step Override:** The workflow template can explicitly define that a certain step *must* use a specific Model/Reasoning Effort, overriding all broader defaults.

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
