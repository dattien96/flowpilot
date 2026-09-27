# CA-1036 — CP-58 review test-coverage batch (gate budget, harness cap, mirror fields, orphan re-drive, switch seed)

Date: 2026-09-26 — deep-review test-coverage findings; no production code
changed. All additions are assertion-based and additive only.

## Tests added

`bug514_reproduce_ask_loop_bound_test.go`:

- `TestBug514_GateRepromptBudgetExhaustedEscalates` — drives the real
  `runFlowGateAtEpoch` path with `repromptAttempts` already at
  `maxFlowGateReprompts`: the gate must block the turn, arm no further
  `pendingGateRepromptPrompt`, and leave the orchestrator loop `blocked`
  via the escalate path (no unbounded reprompt loop). Complements the
  existing reproduce-child `ask_user` budget tests.

`task_harness_cap5_round_reset_test.go` (new):

- `TestTaskHarnessCap5ContinueThenBlockedThenPlanApproveResets` — the
  task-harness `cap: 5` boundary, end to end on the shared Round budget:
  four consecutive `continue` verdicts stay `looping` (Round 1..4), the
  fifth reaches the cap and parks the flow (`awaiting_user` /
  `BlockReason: cap`; tournament rescue may flip Status to
  `tournament_escalation`, which is still parked), and
  `resetPlanPhaseRound` — the plan-approve reset on
  `plan_synthesis --done--> preflight_contract_freeze` — recounts the code
  phase from zero.

`bug458_mirror_node_fields_test.go`:

- `TestRecordFromWorkflowRowBuiltinMirrorRestoresPostureAndContextProfile` —
  pins the last unasserted no-column fields: builtin mirror reconstruction
  restores `posture: read_only` and `contextProfile: scout` on
  vibe-sprint's `preflight_contract_plan` from the embedded pack (the
  CP-62 P-4 structural read-only gate must survive the mirror round-trip).

`bug520_stall_gate_reprompt_child_test.go`:

- `TestBug520_ParkedChildRepromptWipedButCodePathsSurviveAndReDrive` —
  pins the orphan-cure seam for the run-60145 residual: the park wipes the
  armed reprompt intent but preserves `pendingGateCodePaths` (the durable
  re-check seed), and `reinvokeMatchingFlowChild` — the Continue resume
  path — revives a parked `waiting_user_approval` child
  (status→running, activationSeq bump) with the seed intact for the next
  post-turn gate.

`bug466_switch_lazy_writer_test.go`:

- `TestSwitchSeedPromptReachesTargetAdapterAsFirstTurn` — the switch seed
  actually reaches the target provider's `SendTurn` as the new leg's first
  request, carrying the `<previous_conversation>` envelope and the
  transcript marker bytes (previously only `Handoff.Mode` /
  `IncludedTurnCount` were asserted — a healthy-mode response could hide a
  dropped seed).

`chat_reattach_envelope_test.go`:

- `TestReattachEnvelopeProviderAgnostic` table extended with
  `ProviderKeyDevin` — the reattach envelope is provider-agnostic glue and
  the review flagged devin as the only uncovered provider.

## Regression

`go test -count=1 -run 'TestTaskHarnessCap5|TestRecordFromWorkflowRow|TestBug520|TestSwitch|TestReattach|TestBug514'` — green;
full `go test -count=1 ./internal/runner/` — green modulo the pre-existing
environmental failures already recorded (missing provider binaries,
TempDir races, MCP parse noise).
