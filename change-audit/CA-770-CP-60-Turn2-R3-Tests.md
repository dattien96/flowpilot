# CA-770 — CP-60 turn-2 R3 tests: MCP vibe gate + reconstruct

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Additive tests for shared MCP vibe deny/allow and awaiting-lock reconstruct idempotence after turn-2 review
# --->8---

## Debate (turn 2)

A DoDMatrix2 PASS 0 C. B CodeParity2 PASS 0 C, R3 test gaps. C KillReview2 KILL_CLEAN 0 findings.

Consensus: production fail-closed. No C to patch. Close B R3 with additive tests. A's back-edge M is OOS (signatures persist).

## Change

Tests only: `ca770_turn2_r3_test.go`. No production edits.

## Tests

- MCP `tools/call` vibe rejected when only review offered (shared server = Claude/Grok/OpenCode)
- MCP round-trip when `setAllowVibeRequirement`
- tools/list omits vibe until allowed
- reconstruct awaiting-lock + second reconstruct

## Provider impact

**Case 2.** Codex already had `ca769_codex_vibe_gate_test.go`. This CA covers the shared MCP path used by Claude, Grok, OpenCode.

## Will not undo

CA-763..CA-769. Old tests untouched. KR-003 closed items stay closed.
