# BUG-068: Desktop Chat Prompt Skills History Not Persisted In Timeline

## Metadata

- Document ID: `BUG-068`
- Title: `Desktop Chat Prompt Skills History Not Persisted In Timeline`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-053: Desktop Chat Turn Skills Summary Chip](../../08-Task/done/Task-053-Desktop-Chat-Turn-Skills-Summary-Chip.md), [Task-012: Terminal Thinking Process](../../08-Task/done/Task-012-Terminal-Thinking-Process.md)
- Child Documents: `none`
- Related Documents: [BUG-067: Desktop Chat Turn Skills Chip Hidden After Slash Pick](./BUG-067-Desktop-Chat-Turn-Skills-Chip-Hidden-After-Slash-Pick.md), [CA-078: Move Desktop Chat Selected Skills Into Prompt Timeline History](../../../change-audit/CA-078-move-desktop-chat-selected-skills-into-prompt-history.md)
- Replaces: `none`
- Tags: `desktop, bugfix, chat, skills, timeline, history`

## AI Quick View

### Summary

- The selected-skills UI was rendered under the composer instead of under the sent prompt bubble.
- Because the selection lived only in `ChatInput` local state, it disappeared after send and could not remain attached to the prompt history when later prompts used no skills.
- The fix moves selected skill names into the timeline prompt item itself and renders that summary directly below the prompt bubble.

### Current Ask

- Done. Sent prompts now keep their own skill summary in the timeline, and later prompts without skills no longer erase the earlier prompt's skill UI.

### Key Decisions

- `V-1` Treat selected skills as prompt history data once the turn is sent, not as composer-only chrome.
- `V-2` Keep the pre-send picker count in the composer, but remove the separate below-composer summary chip.
- `V-3` Render a compact read-only summary below each prompt bubble when that prompt used one or more skills.

### Constraints

- Desktop normal chat UI only.
- No runner, provider, or contract changes beyond prompt timeline metadata in the renderer.
- Preserve the existing per-turn clear-on-send behavior for selected skills in the composer.

### Open Questions

- Prompt-skill history is preserved in the live desktop timeline for the current session; replay/history restoration was not extended in this slice.

### Source Refs

- `Task-053`
- `Task-012`
- `BUG-067`

## 1. Issue Summary

The desktop chat UI showed selected skills below the composer, but the requested behavior was to show that summary directly under the corresponding prompt bubble in the timeline. The older placement also meant the UI depended on composer state and did not behave like persistent prompt history.

## 2. Parent Links

- impacted coding plan: [Task-053: Desktop Chat Turn Skills Summary Chip](../../08-Task/done/Task-053-Desktop-Chat-Turn-Skills-Summary-Chip.md)
- impacted tech design: [Task-012: Terminal Thinking Process](../../08-Task/done/Task-012-Terminal-Thinking-Process.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot`, normal chat mode
- reproduction steps:
  - select one or more skills for a chat turn
  - send the prompt
  - observe the selected-skills UI under the composer instead of under the prompt bubble
  - send another prompt without any skills
  - observe that the older prompt has no prompt-attached skill history
- frequency: always before the fix

## 4. Expected vs Actual

- expected:
  - each prompt that used skills keeps a visible skill summary directly beneath that prompt bubble
  - sending later prompts without skills does not remove the older prompt's skill history
- actual:
  - selected-skills UI rendered in the composer area
  - prompt history itself did not own the skill metadata

## 5. Impact

- users affected: desktop chat users using skill injection
- workflows affected: direct chat prompt review and historical context inspection
- severity: medium

## 6. Root Cause

- hypothesis:
  - the selected-skills summary was implemented as composer chrome instead of prompt history
- confirmed cause:
  - `ChatInput` stored the selected-skills summary entirely in local component state
  - `sendPrompt` created a prompt timeline item with only prompt text, so the timeline had no skill metadata to render or preserve per prompt
- evidence:
  - `apps/desktop-flowpilot/src/components/ChatInput.tsx`
  - `apps/desktop-flowpilot/src/state/store.ts`
  - `apps/desktop-flowpilot/src/state/timelineReducer.ts`
  - `apps/desktop-flowpilot/src/components/Timeline.tsx`

## 7. Fix Strategy

- `F-1` Extend the desktop `TimelineItem` prompt shape to optionally carry selected skill names.
- `F-2` When `sendPrompt` stages a prompt bubble, persist the selected skill names on that prompt timeline item before composer state is cleared.
- `F-3` Render a compact prompt-level skill summary directly under the prompt bubble in `Timeline.tsx`.
- `F-4` Remove the composer-bottom selected-skills summary chip and its CSS so the prompt history becomes the single source of truth for this UI.

## 8. Validation

- `V-1` `npm run typecheck --prefix apps/desktop-flowpilot`
- `V-2` `npm run build --prefix apps/desktop-flowpilot`
- `V-3` `git diff --check`

## 9. Regression Guard

- tests: desktop typecheck and production build
- alerts: none
- audit checks: GitNexus impact reviewed for `ChatInput`, `sendPrompt`, and `applyTimelineEvent`; all reported LOW risk

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`
- notes left unchanged on purpose:
  - replay/history stream restoration for prompt skill summaries was not extended in this slice
