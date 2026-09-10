# CA-819 — vibe sprint-boundary Continue gate

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: feature
summary: Sprint audit done with plan tasks left parks an ok/cancel Continue form for the next sprint instead of settling done after sprint 1
# --->8---

## Why

Live run-223416 (Branch V snake): sprint 1 audit completed and the flow
settled `done` — Task-905/906 never started, and reopening only offered a
node resume, never the next sprint. Same shape as CA-817 run-635006 (1
sprint then done). The operator asked for one auto flow: after a sprint is
done, show the Continue form for sprint 2 in the same session instead of
forcing exit/reopen. Vibe audits settle via `runAuditNode` direct
`applyFlowControl` (`flow_validate_audit_dispatch.go`), bypassing
`tryAdvanceFlowFromNode`, so the `maybeChainVibeSprint`
(`flow_executor.go:1194`) hook never fired on those runs.

## Change

- New `vibe_sprint_boundary.go`: `maybeParkVibeSprintBoundary` (peek next
  sprint without consuming; stamp audit DONE first per CA-818; park loop
  `blocked/vibe_sprint_boundary` + PendingGate ok/cancel),
  `continueVibeSprintBoundary` (ok / empty Continue: consume + takeNext share
  one critical section, then Start flips running and starts; Budget re-parks
  budget, re-armed lock stays parked, emptied plan settles done; sealed loops
  swallow, never resurrect),
  `declineVibeSprintBoundary` (cancel settles `done`; remaining Tasks stay in
  `08-Task/todo/`), `maybeReparkVibeSprintBoundary` (reopen re-derive from
  audit DONE + plan/index; sealed loops stay sealed).
- `runAuditNode` ready + vibe auto-finalize settles call the park first;
  last sprint / budget / non-vibe / lock-waiting / open-cohort / sealed loop
  keep the old settle. Sealed re-checked after unlock (Stop-wins); open
  cohort defers like `applyFlowControl`.
- `SubmitGateDecision` ok (+`continue` alias, drops nothing — `customText`
  passes through as the sprint note) / cancel (settle-refused restores the
  park, 409); `resumeFlowWithFeedback` Retry/Revise consume the park and
  return a fresh snapshot; consume+take share one critical section so a
  stray chain cannot double-start; `ActiveNode` cleared on continue.
- `stopAgentLoop` clears it (Stop wins); reopen paths
  (`interactive_resume`, chat-history open) re-derive it from audit DONE +
  plan/index; `maybeChainVibeSprint` skips while parked; `runSnapshot`
  serves boundary before resume-confirm (same order as the decision router),
  and resume-confirm skips while boundary is parked (single gate on screen).
- TUI blocked card renders `[Continue]` + sprint copy for the new reason
  (`action_ring.go`, `step_runtime.go`); gate card reuses PendingGate
  ok/cancel with a `sprint N/M (Task-…)` label — no DTO change.
- Same checkout also ticks `CP-60-Test-Steps` for live run-223416
  (V1–V4/V6/G1/G3 ticked with §7 evidence; V5/G2 left open; V7/V8
  unexercised) — verification of the bed, not a second logical change.

## Tests

- `vibe_sprint_boundary_test.go` (new, additive-only): park shape + gate
  copy + audit DONE + peek-no-consume + idempotent re-park; no-park matrix
  (last sprint / dev / budget-isolated 8-of-10 / empty plan); ok → sprint 2
  really starts (index + `vibe-sprint` ref + tdd/coder topology + entry
  child spawned); ok matrix over Claude/Codex/Grok run keys; cancel → loop
  done, index kept; empty Continue (`"continue"`, the Retry chip payload)
  → sprint 2 with no note suffix; note Continue delivers
  `Operator note:` into the entry child prompt (polled); double Continue
  starts once; Stop clears the park and late ok starts nothing;
  invalid option rejected with the park kept; pure note/prompt helpers;
  audit auto-finalize integration (reported shape: boundary, never silent
  done); reopen re-park + sealed loop stays sealed.
- `vibe_sprint_boundary_chip_test.go` (tui/app, new): `[Continue]` chip over
  3 providers + `[Stop]` kept + negative control (other reasons still
  `[Retry]`, never `[Continue]`).
- Commands run green (from `apps/local-runner`):
  `go test ./internal/runner/ -run 'TestVibeSprintBoundary_'`,
  `go test ./internal/runner/ -run 'TestCA814_|TestRun202550|TestCA791_'`,
  `go test ./internal/runner/ -run 'TestCA80|TestCA81|ResumeConfirm|Reconstruct|Checkpoint|OnVibe|CollectVibe|SlicerStartsSprint|JoinsAtTaskSlicer|SlicerAfterIngest|VibeSession|VibeSprintBoundary'`,
  `go test ./internal/tui/app/ -run 'TestSprintBoundary_|TestVibeLock_|TestResumeGate|TestGateDecision|TestGate_'`,
  plus isolated re-runs of timing-heavy freeze/plan-approval/provider-matrix
  tests that flake under full-package load.
- Pre-existing failures left untouched (fail identically on clean tree):
  `TestTryAdvanceFlowFromNodeBailsOnNonDelegateTarget`,
  `TestValidatePassedStillChainsInlineAuditForLegacyEdges`. Timing-heavy
  freeze/plan-approval/provider-matrix tests flake under full-package load
  but pass isolated with and without this change.

## Providers

Agnostic Case 1: zero `providerKey` references in
`vibe_sprint_boundary.go`; park/decision/continue key off workingMode +
plan/index + step status only. The `ok` decision path is matrixed over all
three run keys; `cancel`/Continue share the same key-free branch (single-key
coverage). Provider-touching child turns inside started sprints remain
covered by the existing run-202550 matrix.

## Will not undo

CA-814 vibe missing-key auto-finalize (boundary hooks after it, both settle
points covered). CA-817/818 slicer parks + stamp-before-park. CA-791
join/overlay + CA-783 fallback. BUG-234 advance guards. CA-793 checkpoint
semantics (no commit added on this path). CA-801..816 resume-confirm
semantics (separate flag, separate reason, Stop-fence intact).
