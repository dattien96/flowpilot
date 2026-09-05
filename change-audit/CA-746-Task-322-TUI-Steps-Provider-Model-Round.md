# CA-746 — Task-322: TUI steps panel shows provider+model per step and loop round/cap (Desktop parity)

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-322
change_type: feature
summary: steps sidebar rows render provider/model as an indented dim second line with run-posture fallback (selected fill preserved) and the header shows a round R/C chip from loop state; TUI-only, no backend changes
# --->8---

## Problem (CP-58 live observation #1, 2026-09-05)

- Desktop step cards show provider + model per step and loop round (run-548341: `opencode-go/omen-alpha`, reviewer `grok-4.5`, round 2/3); the TUI steps sidebar showed only step name + agent + status.
- Data already existed server-side (`steps-runtime` per-step + top-level provider/model; agent-graph `LoopState.Round/RoundCap/Cap`) — TUI dropped all of it.

## Change (all `tui/app`, render-only)

- `model.go`: `flowLoopRound/flowLoopCap` + `flowStepsProvider/flowStepsModel` (run posture fallback) fields.
- `step_runtime.go`: `StepsRuntimeMsg` += `Provider/Model` (threaded from `snap` in `cmdRefreshStepsRuntime` — TUI-internal, no API change); `applyAgentGraph` persists round + `Cap ?? RoundCap`.
- `session_panel.go`: pure `formatStepProviderSubline` (indented `    provider/model`, empty when unknown) + `loopRoundChip` (`round R/C`, empty when cap unknown); each step row is followed by its dim sub-line (focused row uses the selected fill on the sub-line so the selection block stays continuous); `stepsSectionTitle` appends dim round chip.
- `app.go` / `history.go`: posture + round/cap stored on `StepsRuntimeMsg`, cleared with steps on `/new` + `/open`. Never uses the TUI session posture (`m.provider/m.model`) as fallback. BUG-355 picker/history/open paths untouched.

## Verification

- 7 new tests (`task322_steps_provider_round_test.go`) all PASS across the claude/codex/grok matrix where applicable.
- Full `go test ./internal/tui/...` PASS, zero pre-existing test edits.
- Manual verify pending (operator): open run-548341 → rows match the Desktop screenshot, header `round 2/3`.
