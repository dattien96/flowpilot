# CA-345: Task-252 FCP provenance fully wired (DONE); Task-249 additional store-contract tests; Task-254 marked done

## Scope

Continuation of the CP-51 closure pass. Wires the durable FCP-marker provenance mechanism found dead in the 2026-07-17 audit, adds the remaining feasible Task-249 tests, and closes Task-254 for real.

## Task-254 — Non-Terminal Dispatch Idempotency Retention → DONE, moved to `done/`

- Added `apps/local-runner/internal/runner/idempotency_retention_test.go`: `TestSnapshot_PostTurnAlsoPinsActiveKey` (proves the real `sessionStateOf` post-turn path pins non-terminal keys, not a hand-rolled equivalent) and `TestSnapshot_RetainedKeysReloadAndShortCircuit` (real disk round-trip via `NewLocalFileSessionStore`, reconstruct, `startTurn` short-circuits to the same prior turnID via durable — not RAM-only — recovery evidence).
- T-1 "authority inversion" (`FindActiveByOuterIntent` wired into `startTurn`) deliberately NOT done: verified it is not gated by any CP-51 §10.1 ledger row owned by this task (`I6`, `Rr4` are retention/pruning only) nor by this task's own acceptance check; replacing the RAM idempotency-map authority (battle-tested through BUG-288 R16-R20) for zero required DOD benefit was assessed as unjustified regression risk.
- CP-51 ledger rows `I6`, `Rr4` flipped to ✅.

## Task-249 — additional store-contract tests (still cannot reach `done` — see below)

- Added to `stop_race_barrier_test.go`: `TestAcceptedRejectsPreSendAndPayloadConflict`, `TestOuterIntentNotClearedWhileSendClaimed`, `TestPostSendCancelRequiresTerminalProof` — pure store-contract proofs (no adapter needed) closing part of the `RC`/`RE`/`TP`/`SC` ledger rows' store-contract half.
- **Finding:** checked which CP-51 §10.1 rows Task-249 actually owns — only `C2` is owned solely by 249; every other referenced row (`C1`, `C2a`, `AE`, `RSF`, `PS`, `TP`, `SA`, `RE`, `SO`, `RA`) is co-owned with Task-255 (the real-subprocess crash-matrix harness, itself still not done). Task-249 structurally cannot reach `done` independently of Task-255 landing — documented in its own §8, not left as a silent gap.
- T-6 (delete `durableIdemReplaySafe`/`durableIntentClearOK`) confirmed permanently blocked by the Task-254 T-1 descope above — recorded as a considered, permanent decision, not an open follow-up.

## Task-252 — FCP Marker Run-ID Binding End-To-End → DONE, moved to `done/`

Wired the mint→persist→restore→verify chain that was previously three separate dead links:

- `apps/local-runner/internal/runner/agent_orchestrator.go` — new `SpawnAgentInput.FCPMarkerProvenanceRunID`.
- `apps/local-runner/internal/runner/interactive_service.go` — `spawnChildRun` stamps it onto the new child's `markerProvenanceRunIDs` under the run-creation lock.
- `apps/local-runner/internal/runner/flow_executor.go`, `flow_validate_audit_dispatch.go` — both real call sites that embed a `flowpilot-fcp` marker via `ComposeFlowCodingPrompt`/`renderFlowContextPromptWithSecret` now pass `pkg.WorkflowRunID` (else `pkg.PackageID`) as the provenance — the exact trustID the marker was minted against.
- `apps/local-runner/internal/runner/local_file_session_store.go` — added the two `Pending*ProvenanceRunID` fields to the on-disk record (previously silently dropped).
- `apps/local-runner/internal/runner/supabase_workflow_store.go` — added `MarkerProvenanceRunIDs` to `sessionRuntimeBlob`; fixed `sessionRuntimeFromState` (the encoder) to actually populate all three provenance fields — they were declared and decoded but never written, so the round-trip was dead on Supabase specifically.
- `apps/local-runner/internal/runner/interactive_resume.go` — **second gap found while testing**: `reconstructRunInternal` never copied these three fields from the loaded session onto the new run at all, so even with persistence fixed, every restart silently lost the binding. Fixed in the same struct literal as the other `Pending*` restores.
- New `apps/local-runner/internal/runner/fcp_marker_replay_test.go` (7 tests): own-run/foreign-run/sibling-without-provenance/recorded-provenance/foreign-secret decision-surface tests against `isFlowContextHandoffWithSecret`, an end-to-end spawn-stamps-provenance test, and a real disk-round-trip-then-reconstruct test.
- CP-51 ledger row `I4` flipped to ✅.
- Known non-blocking gap (documented in Task-252 §8): the `reinvokeExistingFlowChild` validate-retry path (existing-run reprompt, not a fresh spawn) is not wired — the two spawn-based paths that are the actual Task-224/BUG-277 scenario are covered.

## Verification

- `go build ./...`, `go vet ./internal/runner` clean throughout.
- All new/targeted tests pass (Task-254: 4; Task-249: 3 new + 8 from CA-342/344; Task-252: 9).
- Full `go test ./internal/runner/...`: 18 failures, all confirmed pre-existing/environment-dependent or flaky-under-parallel-load (re-verified in isolation) — zero regressions.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: feature
summary: Wire FCP marker provenance mint→persist→restore→verify end-to-end (Task-252 done); close Task-254 idempotency retention tests (done); add Task-249 store-contract tests (structurally blocked on Task-255 for full closure)
# --->8---
