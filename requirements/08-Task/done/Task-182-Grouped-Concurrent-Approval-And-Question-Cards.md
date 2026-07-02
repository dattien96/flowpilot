## Metadata

- Document ID: `Task-182`
- Title: `Grouped Concurrent Approval And Question Cards`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md), [SD-09: Approval Gates Tech Design](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- Child Documents: none
- Related Documents: [BUG-157: Desktop Concurrent Approval Gates Hang The Run](../../09-BugFix/done/BUG-157-Desktop-Concurrent-Approval-Gates-Hang-The-Run.md), [CA-195: Grouped concurrent approval and question cards](../../../change-audit/CA-195-grouped-concurrent-approval-and-question-cards.md)
- Replaces: none
- Tags: desktop, ui, approval, question, chat-ui

## AI Quick View

### Summary

- Follow-up to BUG-157: when several approvals (or several questions) are pending at once, the desktop chat now folds them into one collapsible group instead of stacking N full-size cards.
- Approval groups add a bulk "Approve all" / "Deny all" header action; question groups stay collapsible-only (no generic bulk action, since option sets differ per question).
- Every item inside a group is still individually expandable and independently actionable ("one by one"), reusing the existing `ApprovalCard`/`QuestionCard` components unchanged.
- Extended the BUG-157 array-based pending-state fix to questions (`pendingQuestion` → `pendingQuestions[]`), which the group UI needs to be correct: without it, concurrent questions would hit the same orphaning bug BUG-157 fixed for approvals.

### Current Ask

- Group same-kind concurrent pending items (approvals grouped together, questions grouped together) — not mixed into one combined "ask" group.
- Keep per-item approve/deny (or answer) working inside the group.

### Key Decisions

- `T-1` Grouping scope is by kind, not one mixed group (user-confirmed via clarifying question).
- `T-2` Approval group bulk action is "Approve all" / "Deny all": each sends that item's own `"approve"`/`"deny"` decision value to every item whose `details.decisions` offers it; items with a different decision set are skipped by the bulk action and remain individually actionable.
- `T-3` Question groups have no bulk-answer row — arbitrary/differing option sets per question make a generic "answer all the same" ambiguous. The group is a visual fold only.
- `T-4` Grouping is computed by a pure function (`buildTimelineGroups`) extracted into a new `timelineGrouping.ts` module (no React/store imports) so the fold logic is unit-testable without a DOM/React renderer, mirroring how `timelineReducer.ts` is tested independently of `Timeline.tsx`.
- `T-5` A run of exactly one same-kind item renders as a plain card with no group wrapper — grouping only kicks in at 2+.
- `T-6` `pendingQuestion?: PendingQuestion` was generalized to `pendingQuestions: PendingQuestion[]` (same shape as BUG-157's `pendingApprovals`), and `answer()` now takes an explicit `questionId`, because the group UI can render multiple live `QuestionCard`s at once and each must resolve independently.
- `T-7` Groups render collapsed by default (user-confirmed after visual review); the bulk-action row stays visible in the header even while collapsed.
- `T-8` Grouping is by **consecutive run** in the timeline, not "currently still pending" — this was a correction after initial implementation: grouping by pending-count-only caused the group to dissolve the instant every item resolved (e.g. clicking "Approve all"), bursting every item back out as a separate full-size card, which read as the group "auto-expanding." Grouping by consecutive run means a resolved run stays exactly as folded as it was before resolution — only a different item kind in between (a tool call, assistant text, etc.) starts a new run.

### Constraints

- Fold position: all pending items of a kind are grouped at the position of the **last** pending item in the timeline; earlier ones are absorbed into that group (not rendered twice). Resolved items are excluded from the group and continue to render individually as history.
- GitNexus MCP tools were unavailable in this session; verification was via local TypeScript compilation and `node --test`, not `gitnexus_impact`.
- Full end-to-end browser verification could not be completed: the desktop app's bootstrap requires a local-runner backend on `127.0.0.1:4317` (Go service) that isn't running in this sandbox — confirmed via `net::ERR_CONNECTION_REFUSED` in the preview network log. Verification is unit-test-based only (see Acceptance Check).

### Open Questions

- None outstanding. (BUG-157's open question — generalizing `pendingQuestion` to an array — is resolved by this task.)

### Source Refs

- `apps/desktop-flowpilot/src/components/timelineGrouping.ts` (new)
- `apps/desktop-flowpilot/src/components/timelineGrouping.test.ts` (new)
- `apps/desktop-flowpilot/src/components/Timeline.tsx`
- `apps/desktop-flowpilot/src/components/QuestionCard.tsx`
- `apps/desktop-flowpilot/src/components/ChatInput.tsx`
- `apps/desktop-flowpilot/src/state/store.ts`, `apps/desktop-flowpilot/src/state/timelineReducer.ts`
- User screenshots 2026-07-02: 4 stacked "Approval required" cards (only the latest resolvable) and the follow-up request to combine them into a collapsible group with a single approve/reject action

## 1. Goal

When multiple approval-gate (or question-gate) cards are open at the same time in the desktop chat, show one collapsible group per kind with a bulk action for approvals, instead of N separate full-size cards — while keeping every item individually actionable.

## 2. Parent Links

- coding plan: none directly — UI/state polish on top of the existing approval-gate mechanism
- tech design: `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`
- system spec: `requirements/05-System-Specs/SS-08-Approve-Gate.md`
- specific upstream ids: BUG-157

## 3. Trigger

BUG-157 fixed the hang caused by concurrent approvals being orphaned client-side, but the resulting UI still stacks every pending approval as its own full-size "Approval required" card (visible in the user's screenshot: 4 near-identical PowerShell approval cards). The user asked for these to combine into a single collapsible group with one approve/reject action for all, while still allowing per-item action.

## 4. Exact Change

- `T-1` `apps/desktop-flowpilot/src/components/timelineGrouping.ts` (new): pure `buildTimelineGroups(timeline)` — extracted from `Timeline.tsx` — folds any consecutive run of 2+ approval (or question) items into one `approval-group`/`question-group` node, regardless of whether they're still pending or already resolved; a run of exactly one item is left alone.
- `T-2` `apps/desktop-flowpilot/src/components/Timeline.tsx`: imports `buildTimelineGroups` from the new module; adds `ApprovalGroup`/`QuestionGroup` components (collapsible, collapsed by default — user-confirmed after visual review — following the existing `ToolGroup` pattern) and wires them into the `Item()` switch. `ApprovalGroup` renders a header with "Approve all"/"Deny all" bulk buttons plus every `ApprovalCard` beneath when expanded; `QuestionGroup` renders a header plus every `QuestionCard` beneath, no bulk row.
- `T-3` `apps/desktop-flowpilot/src/styles.css`: added `.card-group*` rules matching the existing `.tool-group-*`/`.card` visual language.
- `T-4` `apps/desktop-flowpilot/src/state/timelineReducer.ts` + `store.ts`: generalized `pendingQuestion?: PendingQuestion` to `pendingQuestions: PendingQuestion[]`, mirroring BUG-157's approval fix — narrowed stale-detection to `turn_completed`/`turn_failed` only, derived `status` from array length, updated `settleHistoryReplayPendingState`, `snapshotRunState`/`restoreRunSnapshot`, and all reset call sites.
- `T-5` `apps/desktop-flowpilot/src/components/QuestionCard.tsx`: `submit(value)` → `submit(questionId, value)` so each card in a group resolves independently.
- `T-6` `apps/desktop-flowpilot/src/components/ChatInput.tsx`: "Action required" banner now checks `pendingApprovals.length > 0 || pendingQuestions.length > 0`.

## 5. Touched Areas

- files: `Timeline.tsx`, `timelineGrouping.ts` (new), `timelineGrouping.test.ts` (new), `QuestionCard.tsx`, `ChatInput.tsx`, `store.ts`, `store.test.ts`, `timelineReducer.ts`, `timelineReducer.test.ts`, `styles.css`
- modules: desktop chat timeline rendering, desktop Zustand store (approval/question pending state)
- routes: n/a (desktop Electron app, no routing)
- tables: none — client-only state, no schema change

## 6. Acceptance Check

- TypeScript compiles clean (`apps/desktop-flowpilot`: `npx tsc --noEmit -p tsconfig.json`).
- `apps/desktop-flowpilot/src/components/timelineGrouping.test.ts` (new, 7/7 pass): single-pending-item renders plain, 3 concurrent approvals fold into one group, resolving every item in a group keeps it grouped instead of bursting back out (the "auto-expand" regression), a mixed resolved+pending run stays one group, a resolved run followed by a later unrelated approval (across a tool call) stays a separate group, approvals and questions group independently, single pending question renders plain.
- `apps/desktop-flowpilot/src/state/timelineReducer.test.ts`: 26/26 pass (includes BUG-157's approval-array tests and the question-array symmetry: `permission_required` while a question is pending no longer orphans it).
- `apps/desktop-flowpilot/src/state/store.test.ts`: 58/59 pass via a manual `node --test` harness (no vitest/jest wired for these colocated test files in this repo); the 1 failure is the same pre-existing `localStorage is not defined` environment gap noted in BUG-157, unrelated to this change.
- Live browser/manual click-through could not be completed — the desktop app's Supabase/local-runner bootstrap hangs in this sandbox (`net::ERR_CONNECTION_REFUSED` to `127.0.0.1:4317`). Recommended for the next developer session with the local runner running.

## 7. Out of Scope

- No change to the underlying approval/question resolution protocol (server-side `interactive_service.go` already resolves per-id — untouched, per BUG-157).
- No generic bulk-answer for question groups — left as individually-actionable only (see `T-3` key decision).
- No mixed approval+question single group — scoping decision is group-by-kind (user-confirmed).

## 8. Completion Notes

- result: implemented, typechecked, and unit-tested; live UI click-through not verified in this session (infra gap, documented above)
- follow-ups: none currently tracked; if a provider ever needs a generic multi-question bulk action, revisit `T-3`
- upstream docs updated: none — SS-08/SD-09 describe the protocol-level approval gate, unaffected by this client-side UI change
