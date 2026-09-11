# CA-769 — CP-60 triple-review delta: Codex vibe gate, TDD signatures, lock validate

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Gate Codex vibe-requirement on vibe flag, require tdd-signatures.md before coder, validate CP paste before write, reconstruct persist probe
# --->8---

## Debate

Three independent reviewers. A: DoD unit-testable PASS. B: FAIL two C (Codex fail-open vibe tool; TDD glob). C: KILL_WITH_FINDINGS 0 C, reconstruct probe gap.

Consensus: B C-findings are real; A under-weighted spawn glob. Live demo stays OOS.

## Change

- Codex `allowVibeRequirement` per thread; reject vibe tool when only review offered.
- Spawn uses `vibeCoderSpawnBlocked` + `hasVibeTddSignatures` (not repo `_test.go`).
- `resumeVibeLock` `RejectNonCP` before `WriteFile`.
- `isVibeTestSourcePath` includes `.test.ts`.
- Additive tests + KR-003.

## Tests

- `ca769_debate_fixes_test.go`
- `ca769_codex_vibe_gate_test.go`

## Provider impact

**Case 2 wiring.** Claude/Grok/OpenCode already gated MCP `allowVibeRequirement`. Codex now matches. Negative Codex test added.

## Residual

Live demo. DefaultRules count in prose is 18 not 22 (do not edit `TestDefaultRules`).

## Will not undo

CA-763..CA-768. Old tests untouched.
