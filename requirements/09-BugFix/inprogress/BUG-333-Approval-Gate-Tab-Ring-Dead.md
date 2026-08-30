# BUG-333: Approval gate mounted but Tab/arrows did not move the ring selection

## Metadata

- Document ID: `BUG-333`
- Title: `Approval gate mounted but Tab/arrows did not move the ring selection`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-30`
- Last Updated: `2026-08-30`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-57-Test-Steps](../../07-Coding-Plan/done/CP-57-Test-Steps.md) (section E re-test), [BUG-331](BUG-331-Opencode-Yolo-Off-Does-Not-Gate-Writes-Default-Allow-All.md), BUG-328 (action ring)
- Child Documents: `none`
- Related Documents: `~/.flowpilot/tui.log` pid 42449 (12:13:49–12:15:18)
- Replaces: `none`
- Tags: `tui, approval-gate, keyboard, action-ring, severity-medium`

## AI Quick View

### Summary

- Operator E1 re-test (opencode gate): the `approval  Approve  Deny  ← → Enter · 1-9` card rendered, but Tab did not move the selection; ←/→ were also tried with no visible effect. `tui.log` proves the keys REACHED `Update` (`Type=tab` ×11, `left`/`right` ×10) and no event storm ran between presses.
- Unit repro through the real `Update` path passed in every constructible state (bare card, populated history/flows/providers/accounts/files, live turn) — the exact live veto could not be reproduced, so no single trigger can be named with certainty.
- The audit found the structural weakness that EXACTLY produces this symptom class: `actionRingKeysActive()` let `collectSuggestions()>0` veto the WHOLE ring (keys AND highlight) even with a blocking card mounted, and the Tab case checked suggestions BEFORE the ring — so any live suggestion source (or future one) silently eats Tab/arrows, and with a card pending Tab could even fall through to the posture-cycle branch and fire a runner call.

### Fix

1. **A blocking card owns the keyboard while the user is not typing** (`actionRingKeysActive`): when `approval/question/gate/attention` is pending, keys are active on empty input regardless of any suggestion source; typing (`inputValue != ""`) hands keys back to the composer so `/approve`-style pickers still work.
2. **Tab routes the ring BEFORE suggestions** (app.go KeyTab): with a card pending the ring cycles first; suggestions only apply when the ring is not active. The posture-cycle fallback is therefore unreachable while a card is pending (no more surprise `GET /client/chat-posture` under a gate).
3. **Diagnostic log**: every Tab with a card pending logs `tab-ring: active=… sugg=… input=… idx=… focus=…` so the next live repro names the exact branch in `~/.flowpilot/tui.log`.
4. Flow-blocked cards (BUG-328 Retry/Stop path) keep the legacy gating — untouched.

## Validation

- `bug333_approval_tab_ring_test.go` (8, additive): Tab cycles + wraps, Shift+Tab back, Tab+Enter dispatches the decision, digit activation (BUG-328 guard), Tab/arrows cycle with ALL suggestion sources loaded, typing keeps the composer (ring untouched), Tab never schedules a posture apply while a card is pending, highlight surface stays visible.
- R1: `Bug328` (21 blocked-flow/SS3/paste tests), `ChatPosture`, `ActionRing` suites green; full `tui/app` package shows exactly the 7 documented pre-existing failures (stash-verified earlier today).
- Operator follow-up: rebuild + re-test E1–E5; if Tab ever dies again, `tui.log` now records the ring branch state.

## Follow-up (same day): TRUE live root cause — chat row cache froze the highlight

The operator re-tested after the keyboard-priority fix and Tab/arrows STILL
looked dead. The new `tab-ring:` diagnostic log (pid 87392) proved the ring
state was cycling perfectly — `active=true sugg=0 input="" idx=0↔1` on every
press — so the failure was purely VISUAL. Reproduced as failing tests this
time: `chatRows()` caches `buildChatRows()` output keyed on `chatRowsSig()`,
which hashes message content + approval/question/attention/blocked identity
but NOT the live ring visual state. Every Tab/←/→ moved `actionRingIdx` while
the cache kept serving the approval bar with the idx=0 highlight baked in —
the selection moved, the screen never repainted it. (The row cache landed
with the CA-5xx scroll-chrome perf work, which is why Tab worked before it.)

Fix: `chatRowsSig` now hashes `actionRingIdx`, `actionRingFocus`,
`actionRingKeysActive()`, gate identity + `AwaitingCustom`, and
`question.Selected` — any keyboard move on any card invalidates the rows and
the highlight repaints. Scroll reuse (TestChatRows_ScrollReusesRowCache)
stays green.

Tests added (TrueColor profile forced — without a TTY lipgloss degrades both
highlight styles to identical plain text, which is ALSO why the first unit
attempts could not see the freeze): approval row repaints on ring move, gate
row repaints on ring move, approval row repaints on keys-active flip.



## Residual notes (separate findings, not fixed here)

- **E2 ordering watch**: live log showed `file_changed` (12:13:57.030) arriving 1ms BEFORE `permission_required` for the same write. Probe J proved opencode does NOT touch disk on deny, so this is a transcript-event ordering artifact of the opencode mapper (the streamed edit diff surfaces as file_changed before the ask), not an actual gate bypass — re-check during E2 that `/deny` leaves no file.
- The transcript "[APPROVAL] … Waiting user…" line is static text; the interactive chips are the live card row — by design (comment at formatApprovalWaitingLine).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-57
change_type: bugfix
summary: Approval ring owns Tab and arrows while a blocking card is pending; suggestions can no longer veto the gate keyboard
# --->8---
