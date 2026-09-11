# CA-838 — Task-335: drift detector + correction ladder behind opt-in flag (CP-23 Phase 2)

# ---8<--- flowpilot:change-ledger
feature_key: runtime-intelligence
source_doc_id: Task-335
change_type: feature
summary: add internal/driftdetect (weighted heuristics 35/30/25/20, half-life decay, 30/60/80 correction ladder) with per-turn telemetry hooks in root+child gates persisting JSONL drift events for Phase 3; FLOWPILOT_ENABLE_DRIFT_DETECTOR default OFF
# --->8---

## Why

CP-23 Phase 2: long sessions drift — apology loops, same-test-fail loops, token burn without file deltas, out-of-scope edits. A deterministic 0-token telemetry aggregator reusing the existing r-scope signal scores each turn and climbs a soft correction ladder (system note → narrowed context → pause for human); never auto-rollback.

## Change

- `internal/driftdetect/` (new, stdlib-only): `EvaluateTurnDrift(history, current)` — out_of_scope_edit +35 (r-scope `ScopeOutOfScopePaths` reused verbatim), repeated_test_failure +30 (same test name current ∩ previous), apology_loop +25 (≥2 consecutive turns; single apology normal), zero_delta_progress +20 (TokensConsumed > 2000 && no file deltas, CP-23 R-2); score capped at 100; half-life decay on clean turns with file deltas (65→32→16). `resolveCorrectionAction` exact thresholds (<30 none / 30-59 note / 60-79 narrow / >=80 pause) + boundary test 29/30/59/60/79/80. `GenerateSystemNote` deterministic. `DriftEvent` carries `StepID` per DOD §9 (Code Guide §11 omitted it; caller fills from TurnResult).
- `runner/gate_hook.go` (+285/−0 additive): `recordDriftTelemetry` at both `applyDodSignals` sites (root + child); per-(service,run) ring history (cap 20) with same-turn dedupe on gate resume; JSONL append to `<workspace>/.flowpilot/workflow_drift_events.json` (Task-336 handoff).
- `runner/interactive_service.go` (+42/−11, Task-334 seam re-expressed with identical flag-OFF behavior): ladder actions consumed one-shot — `inject_system_note` appends the note to the next assembled prompt; `narrow_context` packs via `promptpacker.PackPrompt` with halved floored caps (works even with the budget-packer flag OFF). `pause_for_human` wiring deliberately deferred: EventUserConfirmRequired is bound to the ss-lock confirm backend — emitting it unregistered would strand clients on an unanswerable endpoint; action is recorded on the persisted event + `[drift]` log (drift-specific pause gate = follow-up).
- Env flag `FLOWPILOT_ENABLE_DRIFT_DETECTOR` default OFF → pre-observation no-op, byte-identical. GitNexus impact: runFlowGateAtEpoch LOW, runChildArtifactOutputGateAtEpoch LOW.

## Tests

`driftdetect_test.go`: all 8 Task-335 §10 signatures exact-name + cap-at-100 + note determinism + JSON round-trip + threshold boundaries (12/12). `task335_drift_hook_test.go` (runner): flag-OFF no-op + byte-identity; flag-ON apology×2 → event + injected note (score 45); scope+test-loop → narrow (65, 246KB→3.6KB packed). Task-334 neutrality tests re-run green; flowgate/promptpacker cross-checks green; gofmt/vet clean.

## Providers

Case 1 agnostic: deterministic Go, 0 LLM; token usage read from EventTokenUsageUpdated which every adapter emits; flag-OFF is a pre-observation no-op — identical for Claude/Codex/Grok by construction.

## Prior claims intact

CA-837 (Task-334 packer seam — extended additively, byte-identity re-proven by its own tests), CA-833..CA-836, CA-695, CA-442, CA-441. Phase-3 consumers should dedupe JSONL events by (run_id, turn_id) on gate-resume replays and treat apology_loop as the weakest evidence signal (reviewer notes).
