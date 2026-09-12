# CA-841 — harden drift resume scoring, state eviction, packer seam edges

# ---8<--- flowpilot:change-ledger
feature_key: runtime-intelligence
source_doc_id: Task-335
change_type: bugfix
summary: independent-review hardening — gate re-evaluation of the same turn no longer self-compares (score inflation closed), drift state map bounded with idle eviction, pending ladder note re-stashed on empty-prompt assembly, rune-safe budget truncation, byte-semantics documented
# --->8---

## Why

Fresh-eyes review round 1: (1) on the gate-resume path the stored history still ended with the turn being re-evaluated, so the turn compared against itself — a lone scope-violation turn self-fired repeated_test_failure and inflated 35 → 65/pause; (2) the driftStates map grew without bound across finished runs; (3) a pending drift note was consumed and silently dropped when the assembled prompt was empty; (4) truncateToTokens byte-clips could split a UTF-8 rune; (5) EstimateTokens' byte semantics (conservative for Vietnamese/CJK) were undocumented in code.

## Change

- `runner/gate_hook.go`: recordDriftTelemetry drops trailing history entries matching the current TurnID before evaluating; driftRunState gains lastSeenUnixNano with driftStateMaxEntries=128 / driftStateIdleTTL=1h idle eviction (stalest-first fallback); new restoreDriftLadderActions puts one-shot actions back.
- `runner/interactive_service.go`: the empty-prompt bail re-stashes ladder actions.
- `promptpacker/packer.go`: clipRunes keeps hard clips rune-safe; package comment documents byte-based estimation and its conservative direction.

## Tests

`task335_drift_hook_test.go`: same-turn re-evaluation stays at score 35 / inject_system_note (no inflation); drift state map stays ≤ 128 entries over 138 runs. promptpacker + Task-334/335 suites green.

## Providers

Case 1 agnostic (deterministic Go, 0 LLM).

## Prior claims intact

CA-837/CA-838 — flag-OFF byte-identity re-proven by Task-334's own tests; Task-336's JSONL (run_id, turn_id) dedupe unaffected.
