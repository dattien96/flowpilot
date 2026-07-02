# CA-212: Flow Timeline Sidebar Always Visible Once A Run Has Started

## Summary

Fixed `BUG-175`: the Flow Mode step-timeline sidebar kept vanishing whenever the main run was not literally `running` — while a sub-agent held an approval (main run idle), in the coder→reviewer gap, and after completion. Per user direction, visibility is now existence-based, not status-based: the sidebar shows for the whole lifetime of a started flow run and hides only before the first prompt.

## What Changed

- `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx`: replaced the `runStatus`-gated `visible` predicate (BUG-168) with `isFlowModeRun(chatMode) && (Boolean(mainRunId) || steps.length > 0)`, and removed the now-unused `runStatus` selector. `resetRun` clears both `mainRunId` and `workflowStepRuntime`, so the only hidden case is before a run starts.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Not verified live (the sidebar only renders inside an active flow run, which needs the runner + a provider account) — flagged in `BUG-175` (`V-2`).

## Notes

- Supersedes BUG-168's narrower `running|waiting_approval|waiting_question` predicate.
- Two other observations from the same session — a reviewer prematurely terminating the flow via `submit_review_outcome` (join-barrier bypass, `SubmitFlowControl` → `applyFlowControl` with no cohort gate) and reviewer-cohort approvals not surfacing on the main run — are separate orchestration issues, not addressed by this display-only fix.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-175
change_type: bugfix
summary: Show the Flow-mode step-timeline sidebar for the entire lifetime of a started run (existence-based, not status-based) so it no longer vanishes during sub-agent approvals, the coder-reviewer gap, or completion
# --->8---
