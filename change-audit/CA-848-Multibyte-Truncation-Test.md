# CA-848 — round-3 hardening: make the multibyte truncation test discriminating

# ---8<--- flowpilot:change-ledger
feature_key: runtime-intelligence
source_doc_id: Task-334
change_type: bugfix
summary: round-3 blocking fix — the multibyte truncation regression test cut on a rune LEADING byte (old byte-slice code passed it too); input switched to 7×"ế" (21 bytes, 20-byte cap cuts mid-rune), mutation-verified; estimator/truncate doc comments aligned to byte semantics
# --->8---

## Why

Round-3 review proved the round-2 multibyte test non-discriminating: the input's byte 20 is a rune leading byte, so the pre-clipRunes `content[:maxChars]` code produced valid UTF-8 and passed. The genuine mid-rune case (strings.Repeat("ế", 7): 21 bytes vs 20-byte cap) makes the old code emit invalid UTF-8 ("ếếếếếế\xe1\xba") into a provider prompt.

## Change

- `promptpacker/packer_test.go`: TestTruncateToTokens_MultibyteRuneSafe uses the mid-rune input, asserts valid UTF-8, token budget, exact whole-rune retention, and the ASCII boundary case; mutation-verified (reverting clipRunes fails the test).
- `promptpacker/packer.go`: EstimateTokens/truncateToTokens doc comments aligned to byte semantics; hard-clip comment now describes the actual fallback path.
- `task334_budget_packer_integration_test.go`: stale packTurnPrompt comment → applyBudgetPackerIfEnabled.

## Tests

promptpacker 10/10 green; runner Task-334/335 suites green.

## Providers

Case 1 agnostic (deterministic Go, 0 LLM).

## Prior claims intact

CA-837/CA-841 — flag-OFF byte-identity and seam semantics untouched.
