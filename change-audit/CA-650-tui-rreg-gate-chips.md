# CA-650 — TUI r-reg gate decision chips (desktop parity) + working custom path

## What

Live F3 flow (run-161570): the implement step parked at WAITING_USER_APPROVAL
with an r-reg gate block (`Regressed tests: suite_regressed`, options
`keep-test-fix-code / suggest-requirement-change / custom`), but the TUI gave
the operator nothing to click: the card was plain transcript text ("Options:
…") with NO Continue/Stop chips (an armed gate card suppresses the blocked-flow
bar by design) and no decision chips of its own. Worse, the custom option was
silently broken: the TUI client posted `{"option":"custom"}` without
`customText`, which the runner rejects with 400.

## Why

- `buildGateMessage` rendered the r-reg card as text only; `hitBlockedChrome`
  only maps `[Continue]/[Stop]` and `m.flowLoopBlocked()` returns false while
  `m.gate != nil`, so no chips existed at all for r-reg cards.
- `client.SubmitGateDecision` never sent `customText`; runner
  `SubmitGateDecision` (gate_hook.go) requires it for option=custom.

## Fix

`apps/local-runner/internal/tui/`:

- `client.go`: new `SubmitGateDecisionCustom(ctx, runID, customText)` posting
  `{"option":"custom","customText":...}`.
- `app/app.go`:
  - `gateOptionChip` / `optionChips`: clickable chip labels `[Fix code]`,
    `[Suggest req]`, `[Custom]` (desktop radio parity).
  - `buildGateMessage`: block+options card keeps the legacy `Options:` line
    AND gains a chips line (`[Fix code]  [Suggest req]  [Custom]`).
  - `handleGateInput`: `custom <text>` submits the custom decision with text;
    bare `custom`/`3` arms `AwaitingCustom`; while awaiting, Enter submits the
    typed reason. Number/name typing for the two fixed options unchanged.
  - new `armGateCustom` / `submitGateCustom` / `cmdSubmitGateDecisionCustom`.
- `app/model.go`: `GateState.AwaitingCustom` (custom chip → type reason → Enter).
- `app/mouse.go`: `hitGateChrome` maps chip clicks to `gopt:<option>` (wired
  into `clickTargetAt` between blocked-chrome and question-chrome);
  `dispatchMouseClick` handles `gopt:` → submit, or arm custom.

Continue/Stop remain hidden while an r-reg card is armed (chips replace them).
Provider-agnostic (Case 1).

Will not undo: CA-536 (empty-options escape hatch), CA-545 (no Options line
for non-blocking verdicts), CA-537 (spinner after decision).

## Tests

Additive only — legacy suites untouched.

- `apps/local-runner/internal/tui/app/ca650_tui_rreg_gate_chips_test.go` (new):
  - `TestRregGateChipsClickable` — all three chips hit-test as `gopt:*`.
  - `TestRregGateMessageRendersChips` — card contains `[Fix code]` etc.
  - `TestRregGateChipFixCodeSubmitsAndClears` — click clears gate + dispatches.
  - `TestRregGateTypeNumberStillWorks` — `1` on Enter still submits.
  - `TestRregGateCustomChipThenTypedReason` — chip → type → Enter → custom text.
  - `TestRregGateTypedCustomWithInlineText` — `custom <text>` direct submit.
  - `TestRregGateChipsHideContinueStop` — armed gate ⇒ no blocked-flow bar.

## Verification

- `go test ./internal/tui/app/ -count=1` → green (full suite incl. legacy
  gate/mouse/question tests).
- `go test ./internal/tui/client/ -count=1` → green.
- `go vet ./internal/tui/...` clean.

## Manual (operator tick)

Restart TUI → when an r-reg gate parks implement: click `[Fix code]` (keep
test as source of truth), `[Suggest req]`, or `[Custom]` + type reason + Enter.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-43
change_type: bugfix
summary: TUI r-reg gate card gains clickable decision chips [Fix code]/[Suggest req]/[Custom] (desktop parity) and a working custom path that sends customText (the old client call 400'd on option=custom) — run-161570 parked implement with no approve UI
# --->8---