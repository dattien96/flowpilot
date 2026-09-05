# Task-322: TUI Steps Panel Shows Provider + Model Per Step And Loop Round/Cap (Desktop Parity)

## Metadata

- Document ID: `Task-322`
- Title: `TUI Steps Panel Shows Provider + Model Per Step And Loop Round/Cap (Desktop Parity)`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-05`
- Last Updated: `2026-09-05`
- Parent Documents: [CP-58-Test-Steps](../../07-Coding-Plan/inprogress/CP-58-Test-Steps.md)
- Child Documents: `none`
- Related Documents: [BUG-355](../../09-BugFix/todo/BUG-355-TUI-History-Picker-Stale-Va-Workflow-Open-Mat-Transcript.md) (found during the same live session; TUI surface gaps)
- Replaces: `none`
- Tags: `cli-tui, steps-panel, desktop-parity, observability`
- Feature Keys: `cli-tui`

## AI Quick View

### Summary

- Desktop step cards show provider + model per step and loop round (e.g. run-548341: `opencode-go/omen-alpha`, reviewer `grok-4.5`, round 2/3) — the TUI steps sidebar shows only step name + agent + status.
- The data already exists server-side (`steps-runtime` returns `provider`/`model` per step; loop round/cap in the agent-graph/loop state) — TUI only needs to render it. No backend changes.
- TUI-only change with additive tests; Desktop untouched.

### Current Ask

- Implement T-1 (provider+model per step row), T-2 (loop round/cap chip in steps header), T-3 (unit tests); manual verify against the Desktop screenshot of run-548341.

### Key Decisions

- `T-1` Render-only: no API changes — `steps-runtime` + agent-graph already carry provider/model/round. If any field is absent (old runs), the row falls back to today's rendering (no gaps, no "unknown" noise).
- `T-2` Round display reads the flow loop state (`round/cap`), not per-step data — steps don't own rounds; the header chip shows e.g. `round 2/3`. For completed runs it shows the final round.
- `T-3` Main-chat step cards (Desktop-style inline cards in the TUI transcript) are explicitly OUT — separate, larger work; this task is sidebar-only.

### Constraints

- Additive tests only: no pre-existing TUI test edits (steps panel has history/open/backfill/blocked suites that must stay green).
- No runner/backend changes — if a field is missing server-side for some run kind, degrade gracefully, do not extend the API in this task.
- Must not change picker/history/open behavior (BUG-355 area) — render path only.

### Open Questions

- None blocking. Follow-up (not this task): TUI main-transcript step cards (observation #2 in CP-58 session).

### Source Refs

- CP-58 live session 2026-09-05 (Desktop run-548341 screenshot vs TUI sidebar); `tui/app` steps panel render; `client.GetWorkflowStepsRuntime` (`tui/client/steps_runtime.go`); loop state via agent-graph snapshot (`tui/app/agents_focus.go:549+`, `cmdHydrateAgentGraph`).

## 1. Goal

Opening any flow run in the TUI shows, per step row, the provider + model that ran the step, and a loop round/cap chip in the steps header — matching the information density of the Desktop step cards, with zero backend changes.

## 2. Parent Links

- validation companion: CP-58-Test-Steps §Kết quả (TUI-vs-Desktop observation #1, 2026-09-05).
- TUI client plan: CP-56 (terminal client surfaces).

## 3. Trigger

- Operator comparing Desktop vs TUI on run-548341 could read provider/model/round per step on Desktop but not in the TUI sidebar — slows live validation (CP-58 S/A/B/C sessions).

## 4. Exact Change

- `T-1` Steps panel rows (`tui/app`, steps sidebar render): append provider + model per step from the steps-runtime snapshot (e.g. `plan_writer · doc-writer · opencode/go-omen-alpha`, `reviewer · grok-4.5`). Missing fields → render as today.
- `T-2` Steps header chip: `round R/C` from the flow loop state (agent-graph snapshot; final round for completed runs). Missing state → chip hidden.
- `T-3` Unit tests (new file): rows render provider/model when present + fallback when absent; header chip shows round/cap + hides without state.

## 5. Touched Areas

- `apps/local-runner/internal/tui/app/` — steps sidebar render + header (TUI only).
- New test file(s) under `apps/local-runner/internal/tui/app/`.

## 6. Acceptance Check

- Unit: `go test ./internal/tui/app/ -run 'TestStepsPanelProvider|TestStepsRoundChip' -count=1` PASS (names per implementation).
- Manual: open run-548341 in TUI → every step row shows provider+model matching the Desktop screenshot (plan_writer opencode, reviewer grok-4.5); header shows round 2/3. Screenshot side-by-side vs Desktop.
- Regression: full `go test ./internal/tui/... -count=1` PASS, zero old-test edits.

## 7. Out of Scope

- Main-transcript step cards in TUI (Desktop-style inline cards) — separate enhancement, not this task.
- Any runner/API/store change; any Desktop change; history picker/open behavior (BUG-355).

## 8. Completion Notes

- Empty (implementer fills).
