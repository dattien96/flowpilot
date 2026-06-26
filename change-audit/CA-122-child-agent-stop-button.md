# CA-122: Child Agent Stop Button

## Scope

Added a Stop button to the child-agent chat view in the desktop app so users can interrupt a running child agent without returning to the main chat.

Affected layer: `apps/desktop-flowpilot/src/components/ChatInput.tsx` — `childRunFocused` render branch only.

## Completed

- **ChatInput.tsx** — inside the `childRunFocused ? (...)` branch, added a conditional `<button className="btn send-btn send-btn-stop">` that renders when `blocked` is true (child status is `running | waiting_approval | waiting_question`). The button calls the existing `stop()` store action, which calls `client.interrupt(runId)`. When a child agent is focused via `focusAgentRun`, `store.runId` is already set to the child's run ID, so no new routing logic is needed. The button sits between the input-note wrapper and the existing Main button.
- No changes to `store.ts`, `HttpWsRunnerClient.ts`, `contract.ts`, or CSS.

## Verification

- Code inspection confirms `store.stop()` → `client.interrupt(runId)` where `runId = activeAgentRunId` after `focusAgentRun`.
- Browser preview not run; requires live child-agent running state to exercise the exact render path. Logic is a direct re-use of the proven main-chat Stop button pattern.

## Residual Notes

- Stopping the entire agent loop (`stopAgentLoop`) from the child view is not in scope; that control remains on the orchestration board.
- Follow-up: if the child agent needs a restart-from-child affordance, that should be a separate task under CP-19.

# ---8<--- flowpilot:change-ledger
feature_key: agent-spawn
source_doc_id: CP-19
change_type: feature
summary: Child Agent Stop Button
# --->8---
