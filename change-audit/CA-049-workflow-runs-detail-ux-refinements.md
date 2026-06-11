# CA-049: Workflow Runs Detail Page UX Refinements

## Scope

- Workflow runs details view page (`/workflow-runs/$runId`)
- Steps list sidebar view, run logs card, approval gate interaction, and YOLO status badge in `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`

## Completed

- **Sidebar Step Status Icon**: Updated step sidebar status indicator check to use `isWorkflowStepWaitingForApproval` instead of exact string checks. It now renders the yellow pulsing `ShieldAlert` icon consistently for both `WAITING_APPROVAL` and `WAITING_USER_APPROVAL` states.
- **Log Ordering**: Sorted `allLogs` (and thus `sessionEventLogs`) in descending order by `createdAt` so that the newest logs are displayed at the top of the log cards.
- **YOLO Status Display**: Replaced the interactive Workflow YOLO toggle switch in the header section with a non-interactive/readonly status badge component display showing `YOLO: ENABLED` or `YOLO: DISABLED`.
- **Relocated Approval UI**: Pinned the approval gate card container (showing only Check/Approve and X/Reject icon buttons and the permission/error summary message) to the bottom of the left steps sidebar. Removed the approval box and textarea inputs from the right details workspace footer.
- **Fixed Compilation Errors**: Resolved TypeScript type issues (e.g. `outline` Button variant, missing `size` attribute, and nullability references of `group.session` in nested closure scopes) inside `$runId.tsx`.

## Verification

- Successfully compiled using `npx tsc --noEmit`.
- Ran and passed `npx vitest run workflow-runs` tests.

## Residual Notes

- None.
