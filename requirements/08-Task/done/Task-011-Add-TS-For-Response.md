# Task-011: Add Timestamp For Response

**Phase:** 8
**Depends on:** CP-08

---

## 1. Core Concept

In the workflow run detail page, the prompt chat bubble displays its timestamp, but the response (attempt/output) did not show when the response was received. This task adds the response timestamp (derived from the `createdAt` attribute of the output record) next to the "Attempt N" or "Output" panel header text.

---

## 2. Proposed Changes

### apps/admin-web

#### [MODIFY] [apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx](file:///c:/working/flowpilot/apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx)
- Update `CollapsibleAttemptPanel` component to accept an optional `time?: string` prop.
- Render the `time` value inside the header button beside the title using `font-mono text-[9px] text-muted-foreground/60`.
- In the chronological attempts list rendering, pass `time={formatBubbleTime(attemptItem.output.createdAt) ?? undefined}` to each `CollapsibleAttemptPanel` component.

---

## 3. Verification Plan

### Manual Verification
- View a workflow run detail page and verify that beside the attempt panel headers (e.g. "Attempt 1" or "Output"), a timestamp is displayed.
- Check that the formatting aligns cleanly with the header label.

### Automated Tests
- Run `npm run test` inside `apps/admin-web` to ensure all tests pass.
