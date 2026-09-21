# CA-897 — CP-67 P-1/P-2/P-5 runner: coder-outcome transport, record-only batch, negotiation dispatch

# ---8<--- flowpilot:change-ledger
feature_key: contract-first-tdd
source_doc_id: CP-67
change_type: feature
summary: Wires the runner half of Contract-First Scaffold TDD — submit_review_outcome accepts the coder domain statuses and carries batch_signature_requests through the payload, the bridge buffers a renegotiation batch record-only, signature-locked coder turns are offered the tool, coder completion dispatches the synthesis_negotiation hub with the batch injected into its prompt, and the phase-scoped NegotiationRound cap plus the back-edge shadowing fix keep the existing review loop intact
# --->8---

## Why

The pack layer (CA-895) declared the contract but the runner transport was
incomplete: renegotiate_signatures was rejected by both parsers, the batch
was dropped during face mapping, a buffered batch still advanced the flow,
the continue/forward edge into synthesis_negotiation was dead (no
dispatcher), the new phase back-edge shadowed task-harness's normal
validate→implement review loop, the negotiation phase keyed on a step id
that hub turns never carry, and consumeCoderBatchSignatures had no
production caller. This slice closes each of those gaps.

## Change

- `agent_orchestrator.go`: renegotiate_signatures accepted by
  parseReviewOutcomeInput (batch required in-turn), preserved verbatim
  through reviewOutcomeToFlowControl into FlowControlInput.Payload, and the
  fallback review face maps it to continue; AgentLoopState gains the
  phase-scoped NegotiationRound/NegotiationCap (default 5, B-10).
- `coder_outcome.go` (new): CoderBatchSignatureRequest, the coder face
  (completed→done, renegotiate_signatures→continue, blocked→escalate),
  batch parse/validate, pendingBatchSignatureByStep buffer
  (buffer/snapshot/consume), isSignatureLockedCoderChild, and
  renderNegotiationBatchPrompt.
- `interactive_service.go`: turnBridge.SubmitFlowControl buffers a
  batch-carrying continue record-only (no applyFlowControl) and rejects a
  non-cohort delegate's bare done/continue; the locked-coder offer of
  submit_review_outcome is computed outside s.mu (the helper locks
  internally); applyFlowControl detects the negotiation phase via
  negotiationPhaseActive (activeHubNodeID, since hub turns carry synthetic
  step ids) and consumes NegotiationRound with a hard cap-escalate, never
  the review loop's extend machinery; dispatchHubNotifyNodeWithPrompt lets
  the hub receive the adjudicated batch.
- `interactive_handlers.go`: handleSubmitFlowControl maps coder-domain
  statuses through the coder face before generic parsing, buffers the batch
  from any payload shape after the run-exists check, and stays record-only
  for continue+batch.
- `flow_executor.go`: tryAdvanceFlowFromNode routes a coder completion with
  a pending batch to the edge-declared negotiation hub (a hub.inline node
  targeted by another hub's continue/forward edge — immune to a stale
  activeHubNodeID), consuming the batch into the hub prompt; and
  resolveContinueBackEdgeTarget excludes phase-hub-owned back-edges from
  the hub-aware fallback AND the unscoped first-match, so a normal review
  continue from synthesis still re-enters implement.
- `gate_hook.go`, `scaffold_gate.go`, `reproduce_gate.go`,
  `provider_event.go`, `artifact_type_registry.go`,
  `flow_validate_audit_dispatch.go`: scaffold-turn gate signals (RED proof,
  stub whitelist, signature snapshot), the artifact lock, reproduce-flag
  retirement (B-9: always-on), agent.scaffold as a frozen-writer behavior,
  and scaffold turn completion wiring.
- Tests (additive): cp67_coder_transport_test.go covers parse→map→payload,
  record-only bridge + HTTP paths, delegate done/continue rejection, the
  back-edge shadowing regression pin, negotiationHubNodeFor,
  negotiationPhaseActive via activeHubNodeID, batch prompt rendering, and
  the coder-completion hub dispatch; scaffold_negotiation_test.go pins the
  phase-scoped cap; scaffold_lock_test.go pins the single-version artifact
  lock; scaffold_behavior_test.go the new behavior.

## Known pre-existing failures (verified on HEAD eb2e07f6, NOT caused by
this change)

- TestBUG327_EmitLockedDoesNotUnparkWaitingChild — self-deadlocks: the test
  holds svc.mu while calling agentGraphSnapshot which locks it (broken
  since BUG-367's lock addition, 10/9).
- TestSyncedChatCanResumeAfterServiceRestart — run_not_found.
- TestTryAdvanceFlowFromNodeBailsOnNonDelegateTarget,
  TestIsFlowPlannerExcludedPathCoversSkillpackScaffold — both fail
  identically on clean HEAD.
