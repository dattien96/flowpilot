# CA-353: Re-audit close Task-249 + Task-185; keep 174/178/188 open

## Summary

- **Task-249 → done:** re-audited after Task-255 land. Live V2 prep/claim/linearize/Stop fail-closed + atomic receipt/terminal are production-complete; co-owned crash/Stop DOD mapped to Task-255 B0–B8 + stop_race + store linearizability tests. T-6 permanent descope unchanged. Gemini V2-disabled is matrix policy, not an open gap.
- **Task-185 → done with waiver:** file-level r-contract/r-scope complete; symbol-level SD-21 F-2 and SS-14 E-4 ignore set waived (no AST / no config surface).
- **Task-174 / 178 / 188:** remain `in_progress` with explicit blocking gaps (provenance, UI/subMode, packer T-2 + E2E).

## Verification

- `go test ./internal/runner/ -run "TestStop|TestAccepted|TestOuterIntent|TestPostSend|TestLiveStop|TestDispatchCrashMatrix|TestTerminalCommit|TestOwnRunStop|TestChildSend" -count=1` green (2026-07-17).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-249
change_type: docs
summary: Close Task-249 after Task-255 re-audit; Task-185 done-with-waiver; note 174/178/188 still open
# --->8---
