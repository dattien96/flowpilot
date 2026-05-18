# 1. Human-in-the-loop by default
AI can draft, analyze, suggest, and review. Human approval is required for:
- final product spec,
- final technical spec,
- implementation plan,
- merge decision,
- release decision,
- production rollback decision.

# 2. Optional YOLO mode
Allow any user to enable YOLO mode for a flow, regardless of their role.
If the user wants to dev, fix bugs, or run any workflow with YOLO enabled, we allow it.
In YOLO mode, AI can run the flow automatically without human approval.
In YOLO mode, we save every output artifact, including code diffs, run logs, and decisions, as versioned snapshots under the project.
This makes YOLO mode auditable and helps with debugging later, even if it runs fast.