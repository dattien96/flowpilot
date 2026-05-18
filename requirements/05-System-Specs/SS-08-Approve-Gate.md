# FlowPilot - Approval Gates & YOLO Mode

This document outlines the **Approval Gate** mechanism within FlowPilot, which is a critical feature designed to balance safety, human oversight, and autonomous execution speed.

## 1. Core Concept: Approval Gates
An Approval Gate is a designed pause in a workflow where the system halts execution and waits for human validation before proceeding to the next step. 

For example:
- Before moving from the "Tech Spec Step" to the "Make Plan Coding Step", the system pauses for a Developer or Leader to approve the architectural decisions.
- Before merging code in the "Release Readiness Step", the system waits for a PM to confirm that all acceptance criteria are met.

## 2. Default Safe Mode
By default, the system operates strictly in **Safe Mode**.
- In Safe Mode, transitions between workflow steps are protected by Approval Gates.
- The workflow execution halts and flags its state as `WAITING_USER_APPROVAL` (and can notify the user via the Telegram Notification Step).
- The user must review the generated Artifacts (e.g., the PRD or Tech Spec) and explicitly click "Confirm/Approve" or "Reject/Retry" for the workflow to continue.
- **Value:** This guarantees human-in-the-loop oversight. It prevents AI hallucinations from snowballing into incorrect code, enforces strict architecture reviews, and ensures business intent is perfectly aligned before expensive execution occurs.

## 3. YOLO Mode (Fully Autonomous)
For highly trusted workflows, routine tasks, or rapid prototyping, users can enable **YOLO (You Only Live Once) Mode**.
- When YOLO Mode is enabled, the system bypasses all Approval Gates.
- The AI will execute the entire workflow from start to finish continuously, using its own self-evaluations (like the Code/Review Loop) without ever stopping to ask for human confirmation.
- **Value:** Maximizes speed and automation. This is particularly useful for Solo Developers who want to sprint from idea to code instantly, or for automated background tasks (like Analytics Reviews or Root Cause Investigations) where human intervention mid-flight is unnecessary.

## 4. Configuration & Flexibility
The approval system is designed to be highly configurable:
- **Global Toggle:** Users can toggle YOLO Mode on/off at the Project level.
- **Workflow-Specific:** Specific workflows can be configured to always run in YOLO Mode or Safe Mode, regardless of the project default.
- **Granular Gates:** Even when operating in Safe Mode, users can customize exactly *which* steps require a gate. For instance, a user might require an Approval Gate after the "Architecture Step", but let the "Coding" and "Testing" steps run autonomously.
