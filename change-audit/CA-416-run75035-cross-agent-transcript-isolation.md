# CA-416: run-75035 cross-agent transcript isolation (Codex session steal)

## Summary

Fixed child chat pollution after restart for flow runs: coder (`run-75040`) could show Main hub freeform replies because Codex `refreshResumeHandleLocked` logged the **newest workspace cwd rollout** (often the hub’s) into the **child** turn log, and `seedTranscriptFromDisk` loaded every `codex_session` on resume/focus.

## Root cause

1. **Write path (Codex-specific):** `DiscoverCodexRolloutSessionID(home, cwd)` is not run-scoped; hub+children share `working_directory`.
2. **Read path (shared hardening):** seed trusted every turn-log session id with no parent/sibling ownership filter.

Prior partial fixes (CA-376 desktop orchestration, CA-234 continue while focused, CA-390 hub prefer turn-log, Grok run-536 no-steal) did not cover this child Codex seed shape.

## Changes

- `codex_adapter.go`: record `LastCodexSessionID` from `thread/start|resume`.
- `refreshResumeHandleLocked` Codex: prefer adapter-reported id; if reporter present but empty → no discovery steal; `adapter==nil` legacy discovery still filters foreign-owned ids so multi-run pollution fails closed while old single-run multi-rollout tests stay green.
- `ensureProviderResumeHandle` Codex: prefer run-owned turn-log sessions; children never promote workspace-newest discovery.
- `seedTranscriptFromDisk` / `seedGrokTranscriptFromDisk`: filter foreign parent/sibling session ids before loading files.
- Helpers: `isCodexRealSessionID`, `foreignProviderSessionIDs`, `filterOwnedProviderSessionIDs`, `isForeignProviderSessionID`.

## Tests (additive only)

Go `run75035_cross_agent_transcript_isolation_test.go`:

- refresh refuses foreign hub rollout
- adapter-reported session preferred
- empty reporter does not discovery-steal
- seed ignores parent pollution (markers `done r hả` / `Đúng, đã xong rồi` / `codex-flow-done`)
- seed ignores sibling pollution
- seed keeps own multi-rollout
- hub still loads own session
- preferFlowHubTurnLog still hub-only
- child ensureProviderResumeHandle no steal
- provider parity classification (Codex/Claude/Grok)

Desktop `store.run75035-timeline-isolation.test.ts`:

- orchestration freeform `message_delta` while child focused (codex/claude/grok)
- focus ignores main-tagged freeform
- backToMain restores hub without child-only text

## Verification

- `go test ./internal/runner -run 'TestRun75035_|TestSeedTranscriptLoadsAllCodex|TestRefreshResumeHandle|TestDiscoverCodexRollout|TestRun24377|TestRefreshResumeHandleGrokDoesNotSteal'` PASS
- `npx tsx --test src/state/store.run75035-timeline-isolation.test.ts` 7/7 PASS
- No pre-existing tests edited

## Cross-provider

- Write-path steal: **Codex-specific**
- Seed filter: shared defense; Grok write already no-steal; Claude has no newest-cwd discovery in refresh

## Residual

- Historical polluted turn logs remain on disk; seed filter makes reopen safe without rewriting operator data
- Hub “dense” synthesis display is separate (not treated as bleed)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Stop Codex workspace-newest session steal into child turn logs and filter foreign parent/sibling sessions on seed so hub freeform cannot appear in coder chat after restart (run-75035)
# --->8---
