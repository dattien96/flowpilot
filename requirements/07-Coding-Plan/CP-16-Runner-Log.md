# CP-16: Runner Log System

## 1. Goal

Implement a log system that makes workflow-session behavior observable enough to test the session refactor safely.

The log system must answer these questions from the workflow run detail view:

- did the workflow reuse the main session
- did a step start an isolated subagent session
- did the runner recreate a provider session after failure
- did the runner fall back to one-shot execution
- which provider session id was used for the step

The repo already has `workflow_run_logs`, so this feature does not introduce a new log table. It standardizes what we write into that table and how the UI should interpret it.

---

## 2. Logging Model

Logs stay attached to a workflow step through `workflow_run_logs.workflow_run_step_id`.

Every session-related log entry should be written as a structured message:

```text
session_event:{"event":"session_created",...}
```

This keeps the log readable in the UI and still machine-parseable for testing.

### 2.1 Required Events

The runner/runtime should emit the following session events:

- `session_created`
- `session_reused`
- `session_replaced`
- `session_message_sent`
- `session_dead`
- `session_completed`
- `session_fallback_one_shot`

### 2.2 Required Payload Fields

Each session event should include, when available:

- `workflowRunId`
- `stepRunId`
- `sessionKind` (`main` or `subagent`)
- `provider`
- `model`
- `transportType`
- `providerSessionId`
- `processKey`

For replacement and fallback events, include the reason or failure category.

---

## 3. Placement In The Runtime Flow

The workflow runtime should write logs at these points:

1. when a session is reused from the workflow session row
2. when a new provider session is started
3. when a session is replaced because provider or model changed
4. when a provider message is sent successfully
5. when the runner detects a dead provider session
6. when the runner closes a session before fallback or cleanup
7. when the runner falls back to one-shot execution
8. when the provider session id is persisted or refreshed

The log system must not block workflow execution if a log write fails. Log writes should be best-effort and should never prevent the step from completing.

---

## 4. UI Behavior

The workflow run detail view should continue to show the existing log timeline, but session events must be readable enough to support manual debugging.

Recommended UI behavior:

- show the structured session event text in the existing log stream
- keep chronological ordering stable
- make session-related messages easy to search by prefix
- do not move the workflow UI into a generic chat interface

---

## 5. Implementation Plan

### Phase 1: Structured Session Log Helper

1. Add a helper for structured session log messages.
2. Ensure session logs can be written without breaking workflow execution.

Acceptance:

- session events are emitted as structured `workflow_run_logs` entries
- logging failures do not stop the workflow

### Phase 2: Session Lifecycle Events

1. Log session creation and reuse.
2. Log provider/model replacement.
3. Log successful message sends.
4. Log dead-session recovery and fallback.

Acceptance:

- a developer can inspect the workflow run and see whether the main session or subagent session was used
- a developer can see whether the runner recreated a provider session or fell back to one-shot execution

### Phase 3: UI Verification

1. Confirm the run detail view renders the structured log entries.
2. Confirm the log stream remains chronological after follow-up prompts.

Acceptance:

- session events are visible from the workflow run detail page
- the logs are sufficient to debug the session refactor manually

---

## 6. Definition Of Done Checklist

This feature is only done when all checklist items below are complete.

### 6.1 Storage And Event Format

- [ ] session logs are written to `workflow_run_logs`
- [ ] session logs are attached to the correct `workflow_run_step_id`
- [ ] session logs use a structured `session_event:{...}` prefix
- [ ] required fields are included in session log payloads
- [ ] log writes are best-effort and do not block workflow execution

### 6.2 Session Lifecycle Coverage

- [ ] main-session reuse is logged
- [ ] subagent session creation is logged
- [ ] provider/model replacement is logged
- [ ] successful message send is logged
- [ ] dead-session recovery is logged
- [ ] fallback to one-shot execution is logged
- [ ] session cleanup or completion is logged

### 6.3 Provider Debuggability

- [ ] Codex session reuse/recovery is visible in logs
- [ ] Claude session reuse/recovery is visible in logs
- [ ] Gemini session reuse/recovery is visible in logs
- [ ] provider session id is visible in logs when available
- [ ] process key is visible in logs when available

### 6.4 UI And Testing

- [ ] workflow run detail page renders the new session logs
- [ ] logs stay chronological after follow-up prompts
- [ ] logs remain readable after runner restart or fallback
- [ ] tests cover the structured session event helper
- [ ] tests cover the runtime session lifecycle events
- [ ] manual testing can confirm reuse, recovery, and fallback behavior from the log stream

