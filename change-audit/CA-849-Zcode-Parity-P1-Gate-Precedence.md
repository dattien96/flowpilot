# CA-849 — CP-62 P-1: gate precedence contract between flowgate rules, drift ladder, and vibe owner-debate

# ---8<--- flowpilot:change-ledger
feature_key: zcode-parity
source_doc_id: Task-337
change_type: task
summary: deterministic precedence across the 4 systems sharing the prompt/loop (flowgate rules, CP-23 drift ladder, vibe owner-debate, r-dod settlement) — requirement-class user-only wins; vibe drift ≥80 escalates to owner debate even on a clean gate; r-dod-complete settles before r-requirement; owner-debate turns never get context-pruned (ladder note/narrow dropped at stash + not stashed while debate runs); dev mode byte-for-byte unchanged
# --->8---

## Why

CP-23 (Budget Packer + Drift Ladder), CP-47 (r-dod) and CP-60 (vibe owner-debate) all landed on the same turn loop but had no contract for who wins: drift 80+ hit the deferred (unwired) pause path instead of the vibe resolver; a debate turn could get its violation context shrunk by a pending narrow action; dod-vs-requirement on one turn had no documented settlement order.

## Change

- `flowgate/precedence.go` (new): `ResolvePrecedence` (pure, 0-token) → `PrecedenceResult{Route, Action, Ordered, AllowContextPruning, DriftRouted}`; `PrecedenceRouteUser/OwnerDebate/None`; `VibeDriftDebateThreshold=80`; `IsRequirementViolation`/`IsDODCompleteViolation`. T-4 settlement order: dod-complete first, requirement last.
- `runner/vibe_gate.go`: `classifyVibeGateWithDrift` (drift-aware); legacy 2-arg `classifyVibeGate` kept byte-stable (delegates with 0) so `vibe_gate_test.go` pins stay untouched; `applyVibeGateResolver` uses drift + `startVibeOwnerDebate` (drift-only message fallback); new `latestVibeDriftScore`, `applyVibeDriftOnlyResolver` (clean-gate vibe drift escalation), `startVibeOwnerDebate`.
- `runner/gate_hook.go`: `driftRunState.lastScore` (read by the resolver); clean-gate branch calls `applyVibeDriftOnlyResolver` before passthrough; ladder note/narrow not stashed while the run is inside a vibe-owner-debate flow (T-3).
- `runner/vibe_cp.go`: `stashVibeFlowForDebate` drops pending ladder note/narrow (T-3 — debate assembles with full context).
- SD-20 §7: the precedence table + implementation anchors.

## Tests

`flowgate/precedence_test.go` (9: 5 signatures + below-threshold/warn-only/clean-gate-escalate/empty matrix) and `runner/vibe_gate_precedence_test.go` (7: wiring classify, requirement-beats-drift, drift-only, below-threshold legacy, lastScore read, stash-clears-ladder). Full flowgate suite green; agentpack suite green. Runner full-suite result recorded in the Task-337 completion notes (known pre-existing failures per Task-331 CA convention are unrelated).

## Providers

Case 1 provider-agnostic (pure Go routing over computed violations, 0 LLM) — drift routing lives in the runner gate, not any adapter.

## Prior claims intact

CA-791/CA-793 (vibe checkpoint/join) untouched — `stashVibeFlowForDebate` keeps its park semantics, only adds the ladder drop; Task-335 flag-OFF default preserved (`latestVibeDriftScore` returns 0 with flag off → drift routing inert); BUG-231/BUG-234 settle-state semantics untouched.
