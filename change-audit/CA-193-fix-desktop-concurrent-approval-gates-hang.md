# CA-193 — Fix Desktop Concurrent Approval Gates Hang The Run

## What changed

The desktop chat store tracked a single `pendingApproval` value. When a provider turn issued more than one approval-gated tool call at once (YOLO off), the second `permission_required` event evicted the first from state, and a history-replay stale-detection heuristic (BUG-074) silently stamped the orphaned card "resolved" in the UI without ever submitting a decision to the server. The server-side approval record for that orphaned id stayed blocked forever, hanging the run.

Fix: `pendingApproval` became `pendingApprovals: PendingApproval[]` across `timelineReducer.ts` and `store.ts`. `approve()` now takes an explicit `approvalId` and resolves only that entry; `ApprovalCard` passes its own id. The stale-detection heuristic now only fires on `turn_completed`/`turn_failed` — the one transition that cannot happen live while an approval is genuinely still open — instead of on any subsequent event, so concurrent live approvals no longer get orphaned.

No server-side (Go) changes were needed: `interactive_service.go` already resolves each approval independently by `approvalId`.

## Files touched

- `apps/desktop-flowpilot/src/state/timelineReducer.ts`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/components/ApprovalCard.tsx`
- `apps/desktop-flowpilot/src/components/ChatInput.tsx`
- `apps/desktop-flowpilot/src/state/timelineReducer.test.ts`
- `apps/desktop-flowpilot/src/state/store.test.ts`

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: BUG-157
change_type: bugfix
summary: Track concurrent approvals as a collection so parallel tool-call approval gates no longer orphan and hang the run
# --->8---
