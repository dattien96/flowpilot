# CA-859 — CP-23 ladder completion: dev-mode drift ≥80 parks the run and asks the human

# ---8<--- flowpilot:change-ledger
feature_key: drift-pause
source_doc_id: Task-348
change_type: task
summary: wire the ladder's pause_for_human leg (80+) for real — dev-mode runs park with BlockReason "drift" (loop status blocked, GateReason carrying score/signals/step), emit a drift_pause_required event additively, and resume through the ordinary parked-flow continue/feedback channel so no client can be stranded; vibe mode never asks the user for drift (P-1 owner-debate routing owns it); the arm runs outside st.mu to preserve the documented lock order
# --->8---

## Why

Task-335 deferred the pause leg: emitting the CP-60-style confirm event without a registered ss-lock backend would strand the client modal on an unanswerable endpoint, so drift 80+ only logged. The Operator asked for the leg to be wired for real (2026-09-13): dev mode asks the human at 80+, vibe keeps owner-debate.

## Change

- `runner/drift_pause.go` (new): `armDriftPause(rs, event)` — resolves the governing run (flow children park their parent hub; plain chat runs park themselves), skips vibe entirely, is idempotent while already drift-parked (BlockReason lives in the AgentLoopState, checked via `loopStateFor`), parks through `mutateLoop` (status blocked / BlockReason "drift" / GateReason with score+signals+step) + `parkFlowForAwaitingUser` + graph emit + session persist, and emits `drift_pause_required` with the payload. Resume is the EXISTING parked-run feedback channel (`POST /agent-loop/continue`), the same surface gates and the Task-345 decision card use — no new confirm endpoint, nothing to strand.
- `runner/gate_hook.go`: the ladder case only records the action; `pauseRequested` is captured and `armDriftPause` runs AFTER `st.mu.Unlock()` — never takes s.mu while holding st.mu (the lock-order inversion class fixed in the CP-62 review pass).

## Tests

`runner/drift_pause_test.go` (4, additive): dev park lands (blocked/drift + GateReason + drift_pause_required payload score 86); idempotence (second arm is a no-op, exactly one event); vibe never parks and never emits; flow child drift parks the parent loop. Drift/gate surface `-race` green (`TestDriftPause|TestVibeGate|TestDrift|TestBUG335`).

## Providers

Case 1 provider-agnostic — run-state parking + additive provider event; no adapter involvement.

## Prior claims intact

Task-335 ladder thresholds/scoring untouched (30–59 note, 60–79 narrow legs unchanged, still flag-gated via FLOWPILOT_ENABLE_DRIFT_DETECTOR); CP-62 P-1 precedence untouched (vibe drift ≥80 still routes to owner debate — now doubly guarded inside armDriftPause); ss-lock gate/confirm endpoint untouched (drift pause deliberately does NOT reuse the SS modal); parkVibeRequirement untouched.
