---
id: BUG-443
title: CA-918b modified a pre-existing LSP test file contrary to additive-only rule
status: done
version: 2
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-380, CA-918b]
---

## AI Quick View
- **What**: The BUG-380 implementation edited existing server_manager_test.go without recorded operator approval.
- **Why**: New notification recording was wired into an old helper, not isolated in a new test fixture.
- **Key constraint**: Never alter pre-existing tests without approval; keep their regression signal intact.

## 1. Metadata
- Document ID: `BUG-443`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `lsp-runtime`
- Parent Documents: [BUG-380](../done/BUG-380-Gopls-Initialized-Notification-Never-Sent.md), [CA-918b](../../../change-audit/CA-918-LSP-Initialized-Handshake-And-URI-Scoped-Diagnostics-Wait.md)

## 2. Symptom and Impact
`/safe-fix-contract` and `apps/local-runner/AGENTS.md:81-86` require pre-existing test files unchanged without user approval. `git diff -- apps/local-runner/internal/lsp/server_manager_test.go` shows CA-918b inserted a call to `lspHelperRecordMethod` into existing `lspHelperServe` and added a helper to that old test file. CA-918b calls it an additive test-infra seam; new assertions are in `bug380_initialized_handshake_test.go`. No approval is recorded in this review. Severity: **medium** contract violation, not proven functional regression.

## 3. Reproduction and Evidence
Inspect the diff hunk in existing `server_manager_test.go` (after `out.Flush`). Relevant `go test -count=1 ./internal/lsp` passed during review, but passing cannot waive additive-only. See `safe-fix-contract/SKILL.md:53-73`.

## 4. Acceptance and Verification
Ask operator if an exception is needed; otherwise move instrumentation to a new isolated fixture without editing old tests, compare unchanged baseline behavior. Not changed here.

## 5. Resolution (2026-09-23, CA-935b)

- `lsp/server_manager_test.go` restored byte-identical to HEAD
  (`git diff --exit-code` clean) — recording call + helper removed.
- Instrumentation moved into the additive fixture:
  `TestBUG380HelperProcess` + `bug380HelperServe` + `bug380RecordMethod` in
  `bug380_initialized_handshake_test.go`; the handshake test spawns
  `-test.run=TestBUG380HelperProcess`.
- `go test -count=1 ./internal/lsp` — all green.
