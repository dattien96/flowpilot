# CA-539 — focused completed sub-agent must not keep the Thinking spinner running

## Problem

Opening a done flow chat and then opening one of its sub-agents (run-101411
grok-coder) made the status-line "Thinking" spinner + elapsed clock animate
forever, even though nothing was running. `cmdFocusAgent` always attaches a
listen-only `StreamLive` to the focused child (it is the only way to keep the
child transcript readable after it finished), so a bare `focusStream != nil`
was read as live work.

## Fix (TUI-only)

Added `AppModel.focusedChildLive()` (`agents_focus.go`): a `focusStream` counts
as live work only when the focused run is genuinely non-terminal (status not in
`runStatusIsTerminal`); when the focused run is missing from the hydrate
snapshot it falls back to parent flow/turn liveness (`connStatus` /
`flowHasActiveAgents`).

Replaced the four `m.focusStream != nil` liveness predicates:

1. `workIsLive()` (`app.go`) — spinner/status-line liveness.
2. `shouldPollStepsRuntime()` (`step_runtime.go`) — F2 step auto-poll cadence.
3. `flowLoopBlocked()` (`step_runtime.go`) — a completed child transcript must
   not unblock a parked loop.
4. `settleFlowIfDone()` (`step_runtime.go`) — a done flow still settles to
   ConnIdle/"done" with a completed child open.

A genuinely running focused child still keeps everything live (status
"running" → non-terminal → live).

## Provider impact

Provider-agnostic (Case 1). `focusedChildLive()` takes no `providerKey` and
never branches on one. All matrix tests parameterize Claude / Codex / Grok.

## additive-tests-only compliance

Only a new test file was added
(`run101411_focused_child_liveness_test.go`); no existing test file was
modified. The CA-537 `TestWorkIsLive_Matrix` suite (the contract this must not
undo) is untouched and still green.

## Verification

- `go build ./...` and `go vet ./internal/tui/...` clean; `gofmt` clean for all
  changed files (LF-normalized temp copies; repo files are CRLF).
- New matrix tests green across claude/codex/grok: completed child → not live /
  static status / no F2 poll / loop stays blocked / flow settles; running child
  → live; unknown child falls back to parent flow state.
- Full `go test ./internal/tui/app/ -count=1` green except the two pre-existing
  environmental `TestCmdFocusAgent_*` network failures (confirmed failing on
  the stashed pre-change tree too; Windows connection-reset shape differs from
  the test's expected dial error).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-539
change_type: bugfix
summary: A listen-only focusStream on a completed sub-agent no longer counts as live work — focusedChildLive requires the focused run to be non-terminal (falling back to parent flow/turn liveness), so opening a done child (run-101411 grok-coder) stops the fake Thinking spinner, elapsed clock, and F2 step poll instead of animating forever.
# --->8---