# CA-195 — Grouped Concurrent Approval And Question Cards

## What changed

Follow-up to BUG-157/CA-193: the desktop chat previously stacked one full-size "Approval required"/"Question" card per concurrently pending item. It now folds 2+ pending items of the same kind into a single collapsible group (`approval-group` / `question-group`), with the fold logic extracted into a new pure module `timelineGrouping.ts` so it's unit-testable without React/DOM. Approval groups get a bulk "Approve all" / "Deny all" header action; question groups stay collapsible-only since option sets differ per question. Every item is still individually expandable and actionable inside the group.

This required generalizing `pendingQuestion?: PendingQuestion` to `pendingQuestions: PendingQuestion[]` in the store/reducer (mirroring BUG-157's approval fix) — without it, rendering multiple live `QuestionCard`s at once would hit the same orphaning bug BUG-157 fixed for approvals, since `answer()` previously resolved against a single tracked value instead of an explicit `questionId`.

## Files touched

- `apps/desktop-flowpilot/src/components/timelineGrouping.ts` (new)
- `apps/desktop-flowpilot/src/components/timelineGrouping.test.ts` (new)
- `apps/desktop-flowpilot/src/components/Timeline.tsx`
- `apps/desktop-flowpilot/src/components/QuestionCard.tsx`
- `apps/desktop-flowpilot/src/components/ChatInput.tsx`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/state/store.test.ts`
- `apps/desktop-flowpilot/src/state/timelineReducer.ts`
- `apps/desktop-flowpilot/src/state/timelineReducer.test.ts`
- `apps/desktop-flowpilot/src/styles.css`

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: Task-182
change_type: feature
summary: Fold concurrent approval/question cards into a collapsible group with bulk approve/deny, and generalize pendingQuestion to an array for correctness
# --->8---
