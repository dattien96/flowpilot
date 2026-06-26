# CA-090: Desktop ChatInput Send→Stop Button Transform

## Scope

`apps/desktop-flowpilot/src/components/ChatInput.tsx` and `apps/desktop-flowpilot/src/styles.css`

Feature: when AI is in progress (status `running`, `waiting_approval`, or `waiting_question`), the Send button in the chat composer transforms into a red Stop/Cancel button. Clicking it calls `store.stop()` which was already wired to `client.interrupt(runId)`.

## Completed

### ChatInput.tsx

- Added `StopIcon` component — inline SVG filled square (14×14, rounded corners), matching the Claude app stop icon pattern
- Added `const stop = useStore((s) => s.stop)` selector alongside existing store selectors
- Replaced the static `<button … Send>` (formerly always-disabled when `blocked`) with conditional render:
  - `blocked === true` → `<button class="btn send-btn send-btn-stop" onClick={() => void stop()}>` with `StopIcon`
  - `blocked === false` → original `<button class="btn btn-primary send-btn">Send</button>`

### styles.css

- Added `.send-btn-stop` rule: `background: var(--err)` (#f85149 bright red), `border-color: var(--err)`, `color: #fff`
- Added `.send-btn-stop:hover`: darkens to `#d73a3a`

### No backend changes

The full cancel chain already existed:
- `store.stop()` → `client.interrupt(runId)` → `POST /client/workflow-runs/{runId}/interrupt`
- Go backend: `rs.turnCancel()` → `context.Canceled` propagates to both Claude and Codex adapters
- Backend emits `turn_failed { error: "interrupted by user", recoverable: true }`
- Both Claude and Codex adapters carry `Interrupt: true` capability flag

## Verification

- `npx tsc --noEmit -p apps/desktop-flowpilot/tsconfig.json` → TypeScript: No errors found

## Residual Notes

- The existing "Stop" text button in `RunStatus.tsx:77` (header bar) still exists alongside the new composer stop button — two stop affordances now visible when AI is active. A follow-up task (Task-065 candidate) should hide or remove the header-bar Stop button
- The stop is turn-scoped: it cancels only the current in-flight turn, not the whole run/thread. The run stays alive and the user can send another message after stopping

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: TASK-065
change_type: feature
summary: Desktop ChatInput Send→Stop Button Transform
# --->8---
