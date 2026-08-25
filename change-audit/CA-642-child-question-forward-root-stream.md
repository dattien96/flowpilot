# CA-642 — Child run questions now surface on the flow root stream

## What

A model `ask_user` question raised on a flow-engine CHILD run (planner/tester/
coder) was emitted only on the child run's event stream. The TUI (and any
client subscribing to the root run) saw the flow step flip to
`WAITING_USER_APPROVAL` but never received the question card — no approve /
reject UI, flow hung. Live reproduction: run-260082 — coder asked how to
handle the Subtract → SubtractChecked rename breaking `format.go` +
`calc_test.go` (outside frozen declared_paths); the F2 steps bar showed
"WAITING user" while the timeline had nothing to click.

## Why

`turnBridge.AskQuestion` (the model-driven path used by the `ask_user` MCP
tool across Claude/Codex/Grok) registered the question on the child run and
emitted `user_question_required` only to `b.rs` — the child's stream. Clients
subscribe to the root run stream (`/client/workflow-runs/{runId}/events/stream`),
so the card never arrived. The step status polling (StepsRuntimeMsg) still
surfaced WAITING_USER_APPROVAL via `settleFlowChildStepAwaitingUserLocked`,
making the hang look like a missing approval UI.

`AskWorkflowQuestion` (flow executor question nodes) was already fine: it is
called with the root `parentRunID`, so it lands on the root stream.

## Fix

`apps/local-runner/internal/runner/interactive_service.go`:

- New `flowRootIDLocked` walks a run's `parentRunID` chain to the highest
  `flowEngineDriven` ancestor (handles nested sub-hub children).
- `turnBridge.AskQuestion` now mirrors the same `user_question_required`
  (same QuestionID/prompt/options) onto the flow root run's stream after the
  child emit. `AnswerQuestion` resolves globally by question id
  (`s.questions[rec.id]`), so answering from the main timeline unblocks the
  child bridge — no client-side changes needed.
- Non-flow parents (plain orchestration) are untouched: no extra events.

Provider-agnostic (Case 1): the fix lives in the shared bridge used by the
Claude MCP `ask_user`, the Codex bridge, and the Grok ACP path.

## Tests

Additive only — legacy suites untouched (`interactive_service_test.go`,
`claude_permission_mcp_test.go`, `agent_orchestrator_test.go` all unmodified):

- `apps/local-runner/internal/runner/ca642_child_question_forward_test.go`
  (new):
  - `TestChildAskQuestionForwardedToFlowRootStream` — flow root + coder child;
    question appears on the root stream, registered against the child run,
    answering via `AnswerQuestion` unblocks the child bridge with the choice.
  - `TestNonFlowParentDoesNotReceiveChildQuestion` — plain parent gets no
    mirrored question (no behavior change outside flow-engine trees).

## Verification

- `go test ./internal/runner/ -count=1 -run 'TestChildAskQuestion|TestNonFlowParent|Test.*Question|Test.*Approval|TestInterrupt|TestExpiredQuestion'` → ok.
- `go test ./internal/runner/ -count=1 -run 'TestAgentGraph|TestSpawnChild|TestFlowStep|TestSettleFlow'` → ok.
- `go vet ./internal/runner/` clean.
- Live: answered `q-261012` (run-260082 coder question) via
  `POST /client/questions/q-261012/answer` → `{"status":"accepted"}`, child
  unblocked (pre-fix this path only existed on the child stream).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-43
change_type: bugfix
summary: child-run ask_user questions now mirror onto the flow root stream so the operator gets a card to answer (CA-642)
# --->8---