# CA-863 — Task-351: handoff on the standard boundary path + precedence unification + child-gate drift

# ---8<--- flowpilot:change-ledger
feature_key: sprint-handoff
source_doc_id: Task-351
change_type: task
summary: fix three CP-62 review P1s — (1) sprint handoff now emits on the STANDARD multi-sprint path (maybeParkVibeSprintBoundary writes handoff-sprint-N right after the boundary park lands, pinned to the just-finished index via new emitSprintHandoffAt, killing the N→N+1 attribution race) and the boundary-Continue start injects previousSprintHandoffContext exactly like maybeStartNextVibeSprint; (2) the live vibe classifier consumes flowgate.ResolvePrecedence (SD-20 §7 becomes literally true, no more parallel copy — dod-bucket block/reprompt now counts toward debate routing to match EnforceResult.Action semantics); (3) a vibe flow CHILD with a clean gate at drift >= 80 now escalates through applyVibeDriftOnlyResolver like the root gate
# --->8---

## Why
Review P1s: the handoff chain only fired on exception paths (the standard park→Continue boundary never emitted or injected — the acceptance scenario never ran); ResolvePrecedence was dead code while the live behavior was a near-copy that could silently diverge; clean-gate vibe children never debated on drift.

## Change
- `vibe_sprint_boundary.go`: boundary park → emitSprintHandoffAt(r, index); boundary-continue Start → handoff injected into the entry prompt.
- `sprint_handoff.go`: emitSprintHandoffAt(rs, sprint) pinned-index variant; emitSprintHandoff delegates.
- `vibe_gate.go`: classifyVibeGateWithDrift delegates to flowgate.ResolvePrecedence.
- `flowgate/precedence.go`: dod violations count toward hasBlock/hasReprompt (T-4 ordering unchanged).
- `gate_hook.go`: child clean branch calls applyVibeDriftOnlyResolver.

## Tests
TestHandoffEnrichment_EmitAtPinnedIndex (pinned index vs advanced cursor); TestVibeGatePrecedence_DriftBelowThresholdLegacySemantics reshaped to a real Enforce-produced shape (Action block WITH a block violation — the artificial no-violation shape cannot occur; disclosed here per oracle-rule); full flowgate + runner CP-62 surface green with -race.

## Providers
Provider-agnostic — runner flow orchestration only.

## Prior claims intact
Task-342 verified-state-only emission unchanged; Task-342 best-effort semantics unchanged; dev-mode byte-stability unchanged (mode!=vibe passthrough first).
