# 1. Workflow-First Principle & Chat as an Auxiliary Surface
- **Workflow is primary**: Engineering tasks, feature implementations, and bug fixes must follow structured, gated workflows rather than unstructured, free-form chat.
- **Chat as an auxiliary surface**: Chat Mode (Desktop App & Terminal TUI) is officially supported as an interactive exploratory scratchpad. However, unlike standard ephemeral chatbots, Chat Mode in FlowPilot is anchored by a durable **Chat-SSOT** (`chatId`, leg-based runs, and linearized cross-provider handoff — see SD-26), ensuring complete persistence and traceability.
- Unstructured, unmonitored chat without workflow/gate backing remains prohibited for production engineering changes.

# 2. Workflow is first, not LLM

- We want to build a system around workflows that help teams work better.
- LLMs are just tools that run inside those workflows.
- The real product is the workflow, the context, the history, the approvals, the auditability, and the integration with Git/Jira/Figma/etc.
- We are not building a better ChatGPT. We are building a system that makes AI usage more reliable and structured.

# 3. Every repeated AI task should become a workflow with:
- input schema,
- required context,
- steps,
- model selection,
- skill selection,
- output schema,
- approval gate,
- validation rule,
- persistent history.

# Summary
We will support many flows that help user create software faster and better

For example:
- New Feature Flow
- Bug Fix Flow
- Tech Debt Flow
- Incident Review Flow
- Planning Flow
- Testing Flow
- Release Flow
- etc

We do not want to create a ChatGPT-like interface cause there are many AI tool that do it well. We want to create a workflow-based AI platform that helps teams build better software faster.