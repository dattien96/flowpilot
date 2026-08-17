# BUG-325: TUI chat freezes after a flow gate with empty options (run-103672)

## Metadata

- Document ID: `BUG-325`
- Title: `TUI chat cannot continue after a flow gate — "Gate options: (enter number or name)" re-prints with no options`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-17`
- Last Updated: `2026-08-17`
- Parent Documents: [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- Child Documents: `none`
- Related Documents: [CA-534](../../../change-audit/CA-534-tui-run-sidebar-flow-thinking-paste-history.md), [CA-521](../../../change-audit/CA-521-tui-blocked-flow-awaiting-user.md), [CA-536](../../../change-audit/CA-536-tui-chat-freeze-empty-options-gate.md)
- Replaces: `none`
- Tags: `cli-tui, flow-gate, chat-freeze, regression, severity-high`

## AI Quick View

### Summary

- run-103672: after a flow gate trigger the TUI showed "Gate options: (enter number or name)" with **no options** and every Enter was swallowed as a gate decision that could never match — the operator could not chat again.
- The runner emits `flow_gate_violation` for every gate verdict (`warn`, `reprompt`, `block`), and only `r-reg` violations with options populate `GateOptions`. Warn/reprompt verdicts (and auto fix-code re-drives, `applyGateFixCodeAutoRepromptLocked`) carry **empty** options.
- The TUI armed `m.gate` unconditionally and `processInput` prefers the gate over chat, so an empty-options gate locked the composer forever; `turn_completed` never cleared it.

### Current Ask

- Arm the interactive gate only for a genuine `block` verdict that carries a decision card (`GateOptions` non-empty); surface every other verdict as info without locking the composer, add an escape hatch for option-less gates, and clear stale gates on turn completion.

### Key Decisions

- `D-1` `client.go` `ProviderEvent` gains `Status` (the runner already serializes it on the SSE wire; live + replayed events both carry it).
- `D-2` Arm gate only when `status == "block" && len(GateOptions) > 0`; warn/reprompt/block-without-options show the info message only (CA-534 real-card behavior preserved).
- `D-3` `handleGateInput` escape hatch clears a gate with no options instead of looping the prompt.
- `D-4` `turn_completed`, `TurnDoneMsg`, `turnFinishedMsg` clear `m.gate` so a stale gate can never swallow input once the flow moved on.

### Constraints

- additive-tests-only: new tests in `ca536_gate_empty_options_test.go`; pre-existing gate tests untouched (still green).
- Runner emit behavior intentionally unchanged — Desktop relies on the event for warn/reprompt too.
- Headless mode still treats any `flow_gate_violation` as fatal (no composer to freeze).

### Open Questions

- `Q-1` None. The runner-side `pendingGateBlock` already requires `len(gateOptions) > 0`, consistent with the TUI arming rule.

### Source Refs

- run-103672 (operator report + screenshot: three consecutive empty "Gate options:" prompts).
- Code: `apps/local-runner/internal/tui/client/client.go` (`ProviderEvent.Status`), `internal/tui/app/app.go` (`handleEvent` `flow_gate_violation`, `handleGateInput`, turn-completion handlers), `internal/runner/gate_hook.go` (emit sites).
- Tests: `apps/local-runner/internal/tui/app/ca536_gate_empty_options_test.go`.

## 1. Issue Summary

In the TUI flow chat, a flow gate trigger with no decision options permanently locked the composer. The chat box stayed visible and typing still worked, but pressing Enter re-printed "Gate options:  (enter number or name)" (empty options) because `handleGateInput` could not match any number/name, and the gate was never cleared by turn completion.

## 2. Parent Links

- impacted coding plan: CP-56 (TUI approval/question/gate cards — Task-286)
- impacted tech design: SD-19 (flow gate / agent loop)
- impacted system spec: none directly (TUI layer)

## 3. Environment and Reproduction

- environment: TUI (`flowpilot chat`), flow mode, gate-mode `warn` or auto fix-code re-drive (`reprompt`), or a block without an `r-reg` option card.
- reproduction steps: run a flow that triggers a non-block gate verdict; the "Gate options:" prompt appears with no options; type anything and press Enter; the same empty prompt re-prints and chat stays frozen.
- frequency: whenever a `flow_gate_violation` event carries empty `GateOptions` (warn gate-mode, reprompt re-drive, non-r-reg block).

## 4. Root Cause Analysis

The TUI armed `m.gate` unconditionally on `flow_gate_violation` (`app.go:1342-1350`). `processInput` routes all input to `handleGateInput` while a gate is active (`app.go:2045`). With zero options the decision loop could never match, and no turn-completion path cleared the gate — the composer was locked until `/new`.

## 5. Fix

1. `client.go`: parse `status` on `ProviderEvent` (already on the wire).
2. `app.go` `handleEvent` `flow_gate_violation`: arm the gate only when `status=="block" && len(GateOptions)>0`; otherwise add the info message without locking.
3. `app.go` `handleGateInput`: escape hatch clears option-less gates.
4. `app.go` `turn_completed` / `TurnDoneMsg` / `turnFinishedMsg`: clear stale gates.

## 6. Tests and Verification

- `go build ./...` clean; `go vet ./internal/tui/...` clean.
- `go test ./internal/tui/app/ -count=1` green (excluding two known pre-existing `TestCmdFocusAgent_*` flakes, re-confirmed on the clean baseline).
- `go test ./internal/tui/client/ -count=1` green.
- New: `ca536_gate_empty_options_test.go` (warn/reprompt/block-without-options do not arm; block-with-options still arms; empty-options chat still sends; escape hatch).