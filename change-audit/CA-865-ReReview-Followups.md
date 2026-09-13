# CA-865 — Re-review follow-ups: opencode gating, warn-mode passthrough, ledger corrections

# ---8<--- flowpilot:change-ledger
feature_key: node-isolation
source_doc_id: Task-352
change_type: task
summary: apply the re-review round findings — opencode adapter now clears the yolo auto-approve fast path for gated flow-node postures (the missed one-liner that left decideFlowNodePosture dead for opencode children, closing the SSOT parity claim across all four bridge-backed providers); the unified vibe classifier restores the pre-unification warn-mode behavior (a violation-routed owner-debate downgrades to passthrough when Enforce's warn downgrade is active, while drift-routed debates still escalate — pinned by test); stale wrong-Q-1 comment removed from the desktop reducer branch; ledger attribution corrections (Task-349 commit carries the Task-350 card-chosen-reset hunk; CA-864/Task-352 claim tests authored in the Task-351 commit)
# --->8---

## Why
The second review round confirmed all four fix tasks closed their P0/P1s but flagged one missed provider (opencode), an undeclared warn-gate-mode semantic delta in the precedence unification, one surviving stale comment, and commit/ledger attribution errors.

## Change
- `opencode_adapter.go`: yoloModes also cleared for gated postures (grok-style).
- `vibe_gate.go`: warn-mode downgrade for violation-routed debates only (DriftRouted unaffected); header comment updated.
- `timelineReducer.ts`: reducer-branch comment corrected (answer channel = agent-loop/continue; Q-1 = FlowAwaitingUserCard feedback box).
- New test: TestVibeGatePrecedence_WarnModeDowngradesViolationDebate.

## Known residuals (documented, not fixed — none reopen the closed P0/P1s)
- Desktop answered-state is not replay-safe (no runner re-emit marker); re-tap re-POSTs continue idempotently and rolls back on 422.
- continueFlow silently no-ops (optimistic state sticks) when parentRunId/client.continueFlow missing — card-without-run edge.
- Stop-race repark can overwrite a written handoff with a done-nodes-only version (verified-state-only, narrow window).
- o.resume() keeps a stale BlockReason, so a later drift pause is conservatively suppressed on a manually-resumed run.
- Boundary-park emit has a microsecond stillOwned→emit window (correctly-indexed but data-empty handoff possible; strictly better than the pre-fix never-emit).

## Tests
TestVibeGatePrecedence_WarnModeDowngradesViolationDebate (+ regression surface) green with -race; opencode/tui build green.

## Providers
Opencode now parity-complete with claude/codex/grok for flow-node posture gating; Gemini remains the documented --print residual (no bridge) shared with read-only chat postures.
