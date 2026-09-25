---
id: BUG-440
title: Reprompt carrier loses code paths for partial file events and LSP retries
status: done
version: 2
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-425, CA-928b]
---

## AI Quick View
- **What**: Carried scope can omit original code or disappear before an LSP reprompt.
- **Why**: The stash prefers any nonempty WrittenPaths over complete diff and resets before all checks/commit.
- **Key constraint**: Keep actual code paths through all retries without widening turn-scoped oracle/test diffs.

## 1. Metadata
- Document ID: `BUG-440`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `change-contract`, `context-regression-engine`
- Parent Documents: [BUG-425](../done/BUG-425-Inferred-Contracts-Capture-Near-Empty-Metadata.md), [CA-928b](../../../change-audit/CA-928-reprompt-inferred-contract-original-paths.md)

## 2. Symptom and Root Cause
Root gate has full observed Git diff but stashes only `tr.WrittenPaths` if nonempty, falling back to `changedPaths` *only when empty* (`internal/runner/gate_hook.go:550-565`). `ChangedFiles` originates from provider `EventFileChanged` (`interactive_service.go:9158-9164`), which can be partial. E.g. diff includes `src/calc.go` + audit note, but only audit note emits a file event; carrier loses `src/calc.go`. Child uses the same exclusive fallback (`gate_hook.go:1488-1512`). In addition, root zero-violation path clears carry (`:414-424`) before LSP check (`:431-435`) and contract commit (`:442-467`). `blockTurnForLSPDiagnostics` queues a reprompt without restashing paths (`lsp_hook.go:45-71`). Severity: **high** for scope/metadata regression after retry.

## 3. Reproduction and Evidence
(1) Set diff=`src/calc.go`+audit, ChangedFiles=[audit] → gate reprompt → carrier lacks src. (2) Seed carry=[src/calc.go], make rules pass but LSP emit diagnostic → LSP queues reprompt after carry is nil. BUG-425's current tests seed carry manually and use complete WrittenPaths, not these paths or restart. These are code-path proofs; no new live or failing test run during review.

## 4. Acceptance and Verification
Union observed code diff with confirmed writes, preserve until all checks + commit succeed (or stop/cancel). Additive red tests for mixed partial events, LSP, multi-reprompt, restart/replay and provider differences; check `tr.GitDiff` still drives oracle only for current turn. Not fixed here.

## 5. Resolution (2026-09-23, CA-929b)

- Root gate now populates/reads durable `pendingGateCodePaths`; reprompt stash
  is a union across chained reprompts; carry merges into
  `changedPaths`/`suggestFeatureKeys`/`prepareChangeContract` via
  `mergeCarriedPathsIntoDiff` (`tr.GitDiff` stays turn-scoped so r-tests/r-reg
  do not refire). Carry cleared on pass (clean + warn paths).
- `lsp_hook.go` accepts carried paths so an LSP reprompt rechecks the original
  turn's files; carry restashed after LSP gate processing.
- Tests: `bug439_440_gate_contract_test.go` (6 tests incl. LSP recheck +
  enforce-mode stash). Red before fix.
