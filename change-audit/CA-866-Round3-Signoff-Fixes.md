# CA-866 — Round-3 sign-off fixes: legacy fixture reshape + ledger/doc consistency

# ---8<--- flowpilot:change-ledger
feature_key: zcode-parity
source_doc_id: Task-352
change_type: task
summary: apply the round-3 sign-off findings — reshape the last legacy fixture TestClassifyVibeGate_OwnerDebateOnReg whose violation carried an empty Rule.Action (a shape flowgate.Enforce can never produce; the round-1→2 unification routes on the per-violation action the Enforce always embeds — live vibe routing verified unaffected, same disclosed-reshape pattern as CA-863's sibling); register the sprint-handoff and node-isolation feature keys that CA-860..865 were already using; add Task-344..352 to CP-62's Child Documents; tick the §6 Acceptance Check boxes of Task-345/346/347/348 to match their §9 DoD claims
# --->8---

## Why
Round-3 sign-off sweep (fresh-eyes agent) found one deterministic pre-existing-test failure introduced by the Task-351 precedence unification (empty-Rule.Action fixture), plus three non-blocking consistency gaps (stale CP-62 child list, §6/§9 checkbox contradictions, unregistered feature keys in use).

## Change
- `runner/vibe_gate_test.go`: TestClassifyVibeGate_OwnerDebateOnReg fixture gains `Action: "block"` on the r-reg rule (production shape); disclosed here per the safe-fix contract.
- `change-audit/FEATURE-KEYS.md`: register `sprint-handoff` + `node-isolation`.
- `CP-62-Zcode-Harness-Parity.md`: Child Documents now lists Task-344..352; Last Updated 2026-09-14.
- Task-345/346/347/348 docs: §6 boxes ticked (each verified by the shipped tests + two review rounds; Task-347's trigger semantics per its §8 completion notes).

## Tests
TestClassifyVibeGate_OwnerDebateOnReg green at HEAD; full CP-62 surface green with -race.

## Providers
Docs/ledger only + one test fixture reshape; no runtime code change.

## Prior claims intact
Runtime behavior unchanged (live Enforce violations always embed the configured Rule.Action — evaluate.go); all CA-856..865 code claims re-verified present at HEAD by the round-3 sweep.
