# BUG-264: Flow-Awaiting-User Card Long Text Overflows Without Wrapping

## Metadata

- Document ID: `BUG-264`
- Title: `Flow-Awaiting-User Card Long Text Overflows Without Wrapping`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Scenario 11)
- Child Documents: `none`
- Related Documents: [CA-262](../../../change-audit/CA-262-flow-awaiting-user-card-wraps-long-unbroken-text.md)
- Replaces: `none`
- Tags: `ui, chat-mode, agent-flow-engine, regression`

## AI Quick View

### Summary

- Found live during CP-36 Scenario 11 testing (`run-11120`): the "Needs your decision" card (`FlowAwaitingUserCard.tsx`), shown when the hub escalates/blocks the review loop, displayed the escalation reason text cut off with no visible way to read the rest, even though `.flow-awaiting-user-detail` already has `max-height: 240px; overflow-y: auto` from a prior scroll/overflow fix.
- Root cause: the escalation text can quote raw reviewer/diff output verbatim (e.g. a git diffstat line like `25 +++++++++++++++++++++++++++++`), a single long unbroken token with no spaces to wrap on. Without `overflow-wrap`, a browser does not break such a token, so it overflows the card's horizontal bounds instead of wrapping onto the next line — the vertical scrollbar exists but the actual content is effectively unreadable/off to the side rather than "missing."
- A secondary compounding factor: the element is a plain `<p>` with default `white-space: normal`, which collapses the source text's own line breaks (from a multi-line reviewer report) into a single run-on paragraph, making the already-overflowing text harder to parse even where it does wrap.

### Current Ask

- Long escalation/gateReason text, including raw command output with unbroken long tokens, must wrap and remain fully readable within the card's existing scrollable area.

### Key Decisions

- `V-1` Add `overflow-wrap: anywhere` to `.flow-awaiting-user-detail` so any unbroken token wraps rather than overflowing.
- `V-2` Add `white-space: pre-wrap` to preserve the source text's own line breaks (paragraph/list structure in a multi-line reviewer report) while still allowing normal wrapping.
- `V-3` No change to the existing `max-height`/`overflow-y: auto` — those already worked correctly for normal wrapped text; only the wrapping behavior for long unbroken tokens was missing.

### Constraints

- Scoped to `.flow-awaiting-user-detail` only (`FlowAwaitingUserCard.tsx`'s own escalation-reason paragraph); the sibling Orchestration Board "Blocked" banner (`OrchestrationBoard.tsx`) uses a different, shorter summary line and was not reported as affected.
- No markdown rendering was added — the text still displays as raw text (backticks, `##`, `**` show literally); only the layout/wrapping behavior changed.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/styles.css` `.flow-awaiting-user-detail` (~line 3348).
- `apps/desktop-flowpilot/src/components/FlowAwaitingUserCard.tsx` (the component rendering this element; unchanged by this fix — the defect was CSS-only).
- Live evidence: user screenshot of `run-11120`'s "Needs your decision" card showing a reviewer's report text (including a `git show --stat` diffstat line) cut off mid-line with no visible way to read further.

## 1. Issue Summary

During live CP-36 Scenario 11 testing, a review-loop escalation's reason text (quoting a reviewer's report, including raw git command output) rendered cut off in the "Needs your decision" card with no apparent way to see the rest, despite the card already having vertical scroll support from a prior fix.

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Scenario 11 — found during its Chat-Mode parity testing pass.

## 3. Environment and Reproduction

- environment: Desktop app, Chat Mode Review Loop run that reaches a `blocked` state with a long, detail-rich `gateReason`.
- reproduction steps:
  1. Run a Review Loop to a point where the hub escalates/blocks with a `gateReason` that quotes raw reviewer output containing a long unbroken token (e.g. a diffstat line).
  2. Observe the "Needs your decision" card: the text appears to stop mid-line, with no clearly visible way to read the remainder.
- frequency: deterministic whenever `gateReason` contains an unbroken token wider than the card's ~560px max-width.

## 4. Expected vs Actual

- expected: the full escalation text is readable, wrapping normally and scrolling vertically within the existing 240px-tall box.
- actual: a long unbroken token overflowed the card's width instead of wrapping, and the text's own line breaks were collapsed into one run-on paragraph, making the overflow read as "the text just stops."

## 5. Impact

- users affected: anyone whose review loop escalates with a `gateReason` that includes raw multi-line or command-output-style text.
- workflows affected: display only — the underlying escalation/blocked state and Stop/Continue actions were unaffected; only the reason text was hard to read.
- severity: low-medium — a readability/UX defect, not a functional one, but it directly obscures the information the user needs to make the Stop/Continue decision the card exists for.

## 6. Root Cause

- confirmed cause: `.flow-awaiting-user-detail` (`styles.css`) set `max-height: 240px; overflow-y: auto` (from a prior "fix panel scroll and overflow bugs" commit) but never set `overflow-wrap`/`word-break`, so a `<p>`'s default `overflow-wrap: normal` left long unbroken tokens un-wrapped, and default `white-space: normal` collapsed the source text's real line breaks.
- evidence: user screenshot showing a `git show --stat HEAD ...` style line with a long run of `+` characters that stops abruptly rather than wrapping, immediately followed (once scrolled/inspected) by the textarea and Stop/Continue buttons that DO render correctly below it — confirming the card itself isn't clipped, only this one long line's horizontal overflow was the visible symptom.

## 7. Fix Strategy

- `F-1` `styles.css`: added `overflow-wrap: anywhere;` and `white-space: pre-wrap;` to `.flow-awaiting-user-detail`.

## 8. Validation

- `V-1` CSS-only change; no build/test suite covers rendered text wrapping in this repo. Verified by direct review of the resulting CSS against the reported failure mode (a long unbroken token) — `overflow-wrap: anywhere` is the standard fix for exactly this case, and `white-space: pre-wrap` is the standard way to preserve source line breaks while still wrapping.
- `V-2` Not performed: a live visual re-test against a real escalation with the exact reported long-token content. This is a low-risk, narrowly-scoped CSS addition; the owner should confirm visually on the next live Scenario 11 pass.

## 9. Regression Guard

- tests: none (CSS-only, no existing visual regression tooling in this repo for this component).
- audit checks: `gitnexus_detect_changes()` was not run — GitNexus MCP tools were unavailable in this thread.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: no markdown rendering was added for `gateReason`/escalation text — it remains intentionally plain text; formatting it as markdown is a larger, separate UX decision out of scope here.
