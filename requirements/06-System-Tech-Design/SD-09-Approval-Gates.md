# FlowPilot Tech Design - Approval Gates & YOLO Mode

This document translates `SS-08-Approve-Gate` into technical implementation details.

## 1. UI/UX Design (Frontend - React)
- **YOLO Mode Toggle:** A global switch on the Project Settings and Workflow Settings to enable YOLO mode.
- **Approval Gate UI:** When a step finishes and requires approval, the UI shows a blocking modal or banner: "Waiting for Approval". Includes buttons for "Approve & Continue" and "Reject & Retry".

## 2. State Machine & Realtime (Supabase)
- `workflow_run_steps` states: `PENDING`, `RUNNING`, `WAITING_USER_APPROVAL`, `DONE`, `FAILED`, `SKIPPED`.
- `SKIPPED` is used when a step is disabled (`is_enabled = false`) in the workflow configuration. The Go-runner marks it as `SKIPPED` and moves to the next step.
- When a step completes, the Go-runner evaluates if an approval gate is configured and YOLO mode is OFF.
  - If YES: It updates the state to `WAITING_USER_APPROVAL`. The runner stops polling for this specific workflow.
  - If NO (YOLO is ON): It updates the state to `DONE` and proceeds to mark the next step as `PENDING`.
- The React frontend subscribes to `workflow_run_steps` via Supabase Realtime. When a state changes to `WAITING_USER_APPROVAL`, the UI instantly updates.
- When the user clicks "Approve", the frontend updates the state to `DONE` and marks the next step as `PENDING`, waking up the Go-runner.

## 3. Notifications
- If Telegram MCP is configured, the Go-runner triggers an API call to the Telegram bot upon entering the `WAITING_USER_APPROVAL` state, sending the artifact summary and a link to the dashboard.

## 4. Reject / Retry Flow
When a user reviews an artifact and clicks **"Reject & Retry"** instead of "Approve":
1. The UI prompts the user for a **rejection reason** (required text input).
2. The frontend updates `workflow_run_steps.status` to `PENDING` (not DONE) and stores the rejection reason in `workflow_run_steps.rejection_note`.
3. The Go-runner detects the step is `PENDING` again. It re-assembles the prompt for that step, **appending the rejection note** as additional context:
   ```
   # Reviewer Feedback (Retry)
   The previous output was rejected. Reason: "<rejection_note>"
   Please revise your output to address this feedback.
   ```
4. The AI re-generates the artifact with the feedback incorporated.
5. The new output replaces the previous artifact version (old version is kept in version history).
6. The step enters `WAITING_USER_APPROVAL` again for another review cycle.
7. This loop continues until the user clicks "Approve & Continue".

**Database addition:** `workflow_run_steps` needs a `rejection_note` (TEXT, nullable) column and a `retry_count` (INTEGER, default 0) column.
