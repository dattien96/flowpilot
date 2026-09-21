# CA-899 — CP-67 live-test findings: verdict reprompt, chat-drift exemption, freeze reuse, red-baseline blind

# ---8<--- flowpilot:change-ledger
feature_key: contract-first-tdd
source_doc_id: CP-67
change_type: bugfix
summary: Fixes found by the live Grok 4.5 task-harness run (run-13173, substitute workspace ~/fp-beds/cp67-live): reviewer verdicts lost to deferred-tool in-flight cancel now reprompt the cohort member (cap 2), runner-owned .flowpilot/chats/** bookkeeping is exempt from frozen-scope drift, contract freeze reuse runs before the planner-readonly check so re-drives are idempotent, and gate_blind red_at_capture is downgraded to warn while a frozen contract governs the run (a red suite is the designed intermediate of contract-first TDD). Adds review_verdict_reprompt_test.go + runner_chat_paths_test.go and records the full live evidence matrix in CP-67-Test-Steps.md §11.6/§11.7.
# --->8---

## Why

The first real Grok 4.5 end-to-end run surfaced four transport/gate defects
that unit tests never reached, all now observable in
`fp-beds/cp67-evidence/diag/run-*.ndjson`:

1. `cohort_member_verdict_reprompt` — every Grok reviewer turn ended with its
   `use_tool(flowpilot__submit_review_outcome)` call cancelled in flight
   (`user_cancel` in the grok session history): the model emits final text and
   the deferred tool call in one response, and the CLI discards the queued
   call when the turn ends. Verdicts never reached the bridge, so the
   plan_synthesis/synthesis hub parked on `missing_review_verdict` forever
   (4 consecutive rounds on run-8112).
2. Scope drift false-positive — the runner's own `.flowpilot/chats/*.ndjson`
   turn/dispatch/session logs land inside the child's diff window and were
   counted as coder writes (run-9968 block).
3. Freeze re-drive false-positive — after any downstream park, the
   planner-readonly fingerprint check compared against the flow-start
   baseline, so the earlier round's legitimate scaffold output read as
   "planner changed files" (run-13173 loop).
4. `gate_blind (red_at_capture)` blocked the scaffold turn — for
   contract-first flows a RED suite is the deliverable, and a dirty tree
   pins a stale red baseline permanently (baseline refresh keeps prior truth
   while dirty).

## Change

- `runner/interactive_service.go` — `verdictRepromptCount` on interactiveRun;
  cohort settle with empty machine verdict + verdict-requiring parent now
  reprompts the child (status flips back to running, bounded at 2 attempts)
  instead of appending a verdict-less entry that parks the hub.
- `runner/gate_hook.go` + `runner/bug356_slice_audit.go` — add
  `IsRunnerChatBookkeepingPath` to the runner-owned exemption lists.
- `changecontract/frozen_scope.go` — new `IsRunnerChatBookkeepingPath`
  (`.flowpilot/chats/` prefix only; contracts/settings still drift per
  CA-427).
- `runner/flow_validate_audit_dispatch.go` — `runContractFreezeNode` now
  opens the store and reuses an already-frozen contract for (runID,
  writerStep) BEFORE the planner-mutation fingerprint check; the check only
  guards genuine new freezes. Draft parse/validate error paths unlock the
  freeze lock.
- `runner/gate_blind_hook.go` — `red_at_capture` no longer blocks when the
  run has a frozen contract with DeclaredPaths (contract-first armed);
  still emits warn + metric.
- `agentpack/flow-pack/agents/reviewer.md` — explicit ordering directive:
  invoke `submit_review_outcome` FIRST and await its result before any
  final message (deferred-tool transports cancel tail-attached calls).
- Tests: `review_verdict_reprompt_test.go` (reprompt ×2 then verdict-less
  completion at cap; recorded verdict skips reprompt),
  `changecontract/runner_chat_paths_test.go` (chats exempt; contracts/
  settings/chatsx not).
- `requirements/07-Coding-Plan/todo/CP-67-Test-Steps.md` — §11.6 live
  matrix filled with run IDs + evidence paths; §11.7 lists all live-found
  defects F-1..F-7 and operator notes (prompt field name, continue-vs-done
  park semantics, resume-after-restart recipe).

## Verification

- run-13173 (grok-4.5, task-harness): full pipeline completed —
  freeze → scaffold (stub + RED) → signature lock pinned
  (`locked_signatures=["func Repeat(s string, n int) string"]`,
  `read_only_paths=[textkit/textkit_test.go]`, contract v2) → implement
  (body-only fill) → validate green → reviewer (reprompt fired, verdict
  recorded) → synthesis → audit (tier3 loop for CA+DoD, resolved) →
  `flow_run_complete_done`.
- `go test ./internal/runner -run 'Cohort|Freeze|CP67|CoderOutcome|
  Negotiation|Scaffold|Signature'` — green (1 unrelated pre-existing
  failure `TestIsFlowPlannerExcludedPathCoversSkillpackScaffold`, fails
  identically on HEAD).
- `go build ./...` + `go vet` clean.

## Residual

- Grok still cancels the first in-flight verdict call every round — the
  reprompt absorbs it (observed 4/4 rounds). A grok-side or adapter-side
  ordering fix would remove the retry entirely; tracked as residual, not a
  blocker.
- `synthesis_negotiation` live path proven on run-2870 (batch recorded,
  hub rejected unsupported change); multi-round cap on a live run is
  covered by unit tests only.
- gate-sandbox target workspace remains TCC-blocked; evidence is from the
  substitute bed.
