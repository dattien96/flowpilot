# Task-060: Desktop Chat Question Card Other Submit Behavior

## Metadata

- Document ID: `Task-060`
- Title: `Desktop Chat Question Card Other Submit Behavior`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-055: Fix Claude Ask-User Live Flow](./Task-055-Fix-Claude-Ask-User-Live-Flow.md), [Task-056: Fix Codex Ask-User Live Flow](./Task-056-Fix-Codex-Ask-User-Live-Flow.md), [04-04: Phase 4 Approval Yolo Finalizer](../../10-Refactor/New-System/04-04-Phase4-Approval-Yolo-Finalizer.md), [06-DOD: Verification Checklist](../../10-Refactor/New-System/06-DOD-And-Verification-Checklist.md)
- Child Documents: `none`
- Related Documents: [BUG-073: Claude Cannot Use FlowPilot MCP Servers (Google Drive at Runtime; ask_user in YOLO=on)](../../09-BugFix/done/BUG-073-Claude-Cannot-Use-FlowPilot-MCP-Servers-Google-Drive-And-AskUser-Yolo-On.md), [Task-055: Fix Claude Ask-User Live Flow](./Task-055-Fix-Claude-Ask-User-Live-Flow.md), [Task-056: Fix Codex Ask-User Live Flow](./Task-056-Fix-Codex-Ask-User-Live-Flow.md)
- Replaces: `none`
- Tags: `desktop-chat, ui, question-card, ask_user, other-input`

## AI Quick View

### Summary

- The structured question card was using the selected option as the submitted answer even when the user typed a value into the `Other...` field.
- Single-select options should now submit immediately on click, while the `Other...` path submits the typed text instead of reusing an earlier selection.
- The change stays inside the desktop chat UI and does not alter the runner-side ask/answer bridge.

### Current Ask

- Done. The question card now distinguishes option clicks from manual `Other...` submission, and the regression is covered by a focused unit test.

### Key Decisions

- `T-1` Single-select option rows submit immediately when clicked.
- `T-2` The `Other...` submit path uses the typed text for single-select prompts and preserves multi-select accumulation.

### Constraints

- Keep the change limited to the desktop chat question card flow.
- Do not change the runner-side `ask_user` / `user_question_required` transport contract.
- Preserve the existing approval and retry behavior for all other question flows.

### Open Questions

- None.

### Source Refs

- `Task-055`, `Task-056`, `PP-31`, `T-29`, `T-30`, `T-40`

## 1. Goal

Fix the desktop question card so that a user can either click a listed option and submit immediately, or type a custom answer into `Other...` and have that typed value be the submitted answer.

## 2. Parent Links

- coding plan: [04-04: Phase 4 Approval Yolo Finalizer](../../10-Refactor/New-System/04-04-Phase4-Approval-Yolo-Finalizer.md); [06-DOD: Verification Checklist](../../10-Refactor/New-System/06-DOD-And-Verification-Checklist.md)
- tech design: [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- system spec: [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md)
- specific upstream ids: `Task-055`, `Task-056`, `PP-31`

## 3. Trigger

During the structured question flow, a selected option could win over a typed `Other...` value, which meant the card could submit the wrong answer. The intended UX is that option clicks are immediate, while the manual `Other...` field is the source of truth when the user chooses to type a custom response.

## 4. Exact Change

- `T-1` Split the question-card submission path so single-select option clicks call the answer handler immediately.
- `T-2` Add a small resolver for the manual `Other...` submit path so single-select prompts use the trimmed typed value, while multi-select prompts still preserve the selected options plus any typed value.
- `T-3` Add a focused regression test that proves a typed `Other...` answer wins over a prior selected option in the single-select path.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/QuestionCard.tsx`
  - `apps/desktop-flowpilot/src/components/questionAnswer.ts`
  - `tests/phase1/questionAnswer.test.ts`
- modules: `desktop-flowpilot`
- routes: `none`
- tables: `none`

## 6. Acceptance Check

- Clicking a single-select option submits that option immediately.
- Typing into `Other...` and pressing submit uses the typed value instead of a previously selected option.
- Multi-select behavior continues to preserve selected options and append any typed `Other...` value.
- The focused regression test passes, and the desktop TypeScript check remains green.

## 7. Out of Scope

- No changes to the runner-side `ask_user` bridge or transport contracts.
- No redesign of the question card layout or the surrounding desktop chat shell.
- No broader workflow or approval-gate policy changes.

## 8. Completion Notes

- result: The question card now matches the intended interaction model for structured questions, with single-select options behaving as direct answers and `Other...` behaving as the explicit manual answer path.
- follow-ups: None.
- upstream docs updated: `Task-060`, `walkthrough.md`.
