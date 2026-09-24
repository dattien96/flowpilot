# CA-963 — KR-005 remaining LOW-gap tests + CP-71-Test-Steps + manual-checkbox annotations

## Summary

Final gap-closure batch of the KR-005 retrospective audit for
CP-71/81/82/83/84. Three deliverables:

1. **Missing `CP-71-Test-Steps.md` written** — every other CP had one.
   Follows the CP-84 format: automated table with exact test names,
   manual prep, §4 desktop UI checks (kept unchecked — operator pass
   pending), §5 live matrix (all 8 items already verified in the KR-005
   live round on `:4317`).

2. **LOW-gap tests added** (all additive, no existing test touched):
   - `cp71_worktree_low_gaps_test.go` — 71-h `merge_failed` on missing
     base sidecar (500, not parked `merge_pending`), 71-i `.gitignore`
     written exactly once across runs over HTTP, 71-k provider-cwd parity
     (`workspaceCwd == worktree path` for claude+codex — provision is
     provider-agnostic by construction, evidence for R2), 71-l
     `markChatWorktreeState` propagates to every leg's binding AND every
     persisted `ProviderSessionState` row.
   - `manager_gitignore_test.go` (internal/worktree) — `ensureGitignore`
     append-when-missing / no-trailing-newline / idempotent.
   - `cp84_projector_purity_test.go` — 84-g: projector under `s.mu` is a
     pure read — deterministic output, source records byte-identical,
     fingerprint stable.
   - `cp84_provider_parity_test.go` — 84-o: extends projection parity to
     all six providers (claude/codex/grok/gemini/opencode/devin) without
     editing the existing 3-provider test.

3. **Manual-checkbox annotations** across CP-81/82/83/84 Test-Steps:
   each remaining `[ ]` now states whether it is operator-required (and
   why it cannot be automated) or already covered by automation. Prep
   rows verified during the KR-005 live round are ticked with evidence.

## Deferred (documented, not fixed)

- 84-q soak test, 83-f mutation-guard, 82-c mid-render race — LOW
  severity, need load/infra harness, not cheap unit tests.
- Desktop UI operator passes (CP-82 M-1..M-7, CP-83 M-2..M-7, CP-84
  M-1..M-11, CP-71 M-1..M-10) — require a real desktop session.

## Changes

- `internal/runner/cp71_worktree_low_gaps_test.go` — new (4 tests).
- `internal/worktree/manager_gitignore_test.go` — new (3 tests).
- `internal/runner/cp84_projector_purity_test.go` — new (1 test).
- `internal/runner/cp84_provider_parity_test.go` — new (1 test, 6 providers).
- `requirements/07-Coding-Plan/done/CP-71-Test-Steps.md` — new.
- `requirements/07-Coding-Plan/done/CP-81-Test-Steps.md`,
  `CP-82-Test-Steps.md`, `CP-83-Test-Steps.md`, `CP-84-Test-Steps.md` —
  checkbox annotations only.

## Tests

- `go test ./internal/runner ./internal/worktree` — green.
- Old tests untouched (safe-fix-contract R1); provider parity explicit
  (R2 evidence); repro + near-miss + degraded paths covered (R3).
