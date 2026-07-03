# CA-234: Awaiting-User Card Follow-Up Fixes (Width + Child-Focus Leak)

## Summary

Live testing of BUG-231's new Continue/Stop awaiting-user card surfaced two regressions introduced by that card's own code (a third and fourth issue observed in the same session were pre-existing/unrelated and are tracked separately in BUG-233, not fixed here). Both are fixed as direct amendments to BUG-231.

## What Changed

### Desktop (`apps/desktop-flowpilot`)

- `components/FlowAwaitingUserCard.tsx`: the feedback `<textarea>` sat outside `.other-row` (the button row), so it never picked up `.text-input`'s `flex: 1` expansion and rendered at the browser's default width. Bumped `rows` from 2 to 4.
- `styles.css`: new `.flow-awaiting-user-feedback` rule (`width: 100%`, `box-sizing: border-box`, `display: block`, `resize: vertical`) so the textarea fills the card regardless of its flex context.
- `state/store.ts`: `continueFlow` called `client.continueFlow` unconditionally, without the child-focus guard `sendPrompt`/`stop` already have — so continuing while a reviewer child agent was focused streamed the resumed hub turn into both the main and the focused child's transcripts. Added the same `activeAgentRunId !== parentRunId` check used by `stop`, calling `backToMainRun()` first so the resumed turn lands only in the main transcript.
- Tests (`state/store.test.ts`): new `"continueFlow returns to the main run before resuming when a child agent is focused (BUG-231 follow-up)"`.

### Docs

- `requirements/09-BugFix/done/BUG-231-...md`: new `## 13. Follow-Up Fixes From Live Testing` section recording both fixes and linking out to `BUG-233` for the two deferred, unrelated issues (step-timeline staleness; blocked-card diagnostic content).
- `requirements/09-BugFix/todo/BUG-233-Flow-Timeline-Staleness-And-Blocked-Card-Diagnostic-Content.md` (new): investigation-only doc for the two deferred issues, with the decided fix direction for the content-mismatch bug (show a concise, reviewer-findings-derived summary instead of the CA-226 internal diagnostic sentence) recorded as `D-1`; no code changed for either issue in this note.

## Verification

- `npm run typecheck` (desktop) — clean.
- `npx tsx --test src/state/store.test.ts` — 70 tests, 68 passed, 2 failed (identical pre-existing baseline: `localStorage` unavailable in the Node test runner, one timing-sensitive `sendPrompt` test — both predate this change). All new/BUG-231-related tests pass, including the new child-focus regression test.
- Not executed: a live click-through in the running desktop app (same sandbox limitation as CA-233 — no reachable local-runner backend). The fixes were verified by direct code/CSS review against the two reported failure modes plus the new unit test for the store-level behavior.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-231
change_type: bugfix
summary: Fix awaiting-user card textarea width and stop Continue from leaking the resumed turn into a focused child agent's transcript
# --->8---
