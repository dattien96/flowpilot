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
To maximize efficiency and leverage the specific strengths of different AIs, providers can be configured at two levels:
- **Workflow Level:** A user can select a single provider to run the entire workflow (e.g., "Run this entire feature flow using Gemini").
- **Step Level (Granular override):** Each individual step inside a workflow can be assigned its own specific provider.
  *Example:* A user can configure the "Make Plan Coding Step" to use **Codex** (for deep architectural reasoning) but configure the "Code/Review Loop Step" to use **Gemini** (for fast code generation and test execution).

---

## 2. Model Configuration

Just as providers vary, the specific model versions under those providers must be highly configurable by the user.

### 2.1 Model Selection
Each installed provider exposes multiple models (e.g., `codex-5.5`, `codex-5.3`, `gemini-1.5-pro`, `gemini-1.5-flash`). The user must be able to select the exact version they want to use.

### 2.2 Model Assignment per Workflow & Step
Model configuration strictly follows the same flexible rules as provider assignment:
- **Workflow Level:** A user can define a single model for the entire workflow (e.g., "Use `codex-5.5` for the whole flow").
- **Step Level (Granular override):** A user can override the model on a per-step basis to balance intelligence and cost/speed.
  *Example:* A user can configure the planning and architecture steps to use the highly capable `codex-5.5`, but configure the actual implementation/coding step to use the faster/cheaper `codex-5.3`.

---

## 3. Configuration UI & Resolution Rules

To support this granular flexibility without confusing the user, the configuration resolves in a specific priority order. We need to let users configure this effortlessly:

1. **Project Default:** The user sets a baseline Provider and Model for the entire project.
2. **Workflow Default:** When triggering a workflow, the user can override the Project Default for that specific run.
3. **Step Override:** The workflow template can explicitly define that a certain step *must* use a specific Provider/Model, overriding all broader defaults.
