# FlowPilot - Workflow Skills and Agents

This document defines the execution layer of the FlowPilot orchestrator, which relies on two core components: **Skills** (the "how-to" instructions) and **Agents** ( the specialized executors).

---

## 1. Skill Registry & Architecture

Skills act as reusable rules, instructions, and checklists that guide the AI on *how* to perform a specific task. They ensure the AI behaves deterministically rather than relying on ad-hoc prompts.

### 1.1 Types of Skills
Skills are categorized into two types:
- **Built-in Skills:** Mandatory, core skills provided natively by the system (e.g., Coding Skill, Architecture Skill, Planning Skill, Testing Skill).
  - Every core workflow step enforces the use of its corresponding built-in skill (e.g., the "Make Plan Coding Step" *must* include the "Planning Skill").
  - Users *cannot* delete a built-in skill from a step, ensuring the system maintains baseline quality and structure.
- **Custom Skills:** User-created skills tailored to specific organizational needs or project domains.
  - Users can create and attach custom skills to any workflow step to augment the existing built-in skill.

### 1.2 Skill Execution Rules
- A skill can be added to any step in a workflow. A single step can utilize multiple skills (1-to-N mapping).
- Each skill's execution can be strictly configured to run autonomously (YOLO mode) or require human approval.
- **Registry Responsibilities:**
  - Store reusable rules and checklists.
  - Version control skill definitions.
  - Attach skills to specific workflow steps.
  - Prevent silent behavior changes by keeping prompt instructions deterministic.

### 1.3 Skill Storage & Syncing
Because FlowPilot supports three different AI providers (Gemini, Claude, Codex) that rely on local CLI tools, the skills must be stored securely within the project directory:
- **Local Storage:** Skills are automatically saved into provider-specific hidden directories in the project root: `.claude/`, `.codex/`, and `.gemini/`.
- **Custom Skill Portability:** 
  - Custom skills are created per-project.
  - Users can sync, backup, and restore custom skills globally using the **Driver Google MCP**.
  - Users can freely export custom skills to seamlessly reuse them across different projects.

---

## 2. Agent Runtime

While Skills provide the instructions and rules, Agents are the specialized personas that *execute* those instructions. 
*(Note: Currently, the MVP only supports **Built-in Agents**; custom agents are not permitted.)*

### 2.1 Agent Responsibilities
The Agent Runtime is responsible for the actual "thinking and doing". Its core duties include:
- Operating as specialized AI roles (e.g., Coder Agent, Reviewer Agent).
- Constructing the final prompt dynamically by combining: `System Instruction + Skill Rules + MCP Context + Task Input`.
- Calling the LLM through the designated Provider Adapter (Gemini, Claude, or Codex).
- Parsing the LLM's response and returning a structured artifact output back to the workflow orchestrator.

### 2.2 Agent Execution & Sub-Processes
- A workflow step is assigned a specific Agent if the task requires a specialized, heavy-duty execution environment. 
- *Example:* The "Coding Step" requires the "Coder Agent". This agent acts autonomously and runs in a separate background process, allowing it to write code, compile the project, and run tests recursively.
- **Go-Runner Integration:** When a workflow step utilizes an Agent, the orchestrator passes the Agent's definition down to the backend Golang engine. The backend then correctly initializes the `go-runner` tool to manage and isolate the Agent's separate execution process.
