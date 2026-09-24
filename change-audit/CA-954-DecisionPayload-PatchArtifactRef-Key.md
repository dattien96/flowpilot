# CA-954 — KR-005 CP-84: patchRef always empty (producer/reader key mismatch)

## Summary

New payload-content tests (`cp84_decision_payload_gaps_test.go`) caught a real
bug: the worktree-merge producer emits `patchArtifactRef`
(`run_worktree_merge.go`) but `worktreeDecisionPayload` only read `patch_ref`
/`patchRef` — so the `patchRef` field in every emitted worktree_merge decision
payload was always empty; the desktop never received the artifact ref.

Fix: reader now accepts `patchArtifactRef` too (kept `patch_ref`/`patchRef`
for compat). New tests also cover gate bounded-context truncation, question
options propagation, and dispatch decision payloads — the previously untested
payload kinds.

## Files

- `apps/local-runner/internal/runner/decision_payload.go` — accept
  `patchArtifactRef`
- `apps/local-runner/internal/runner/cp84_decision_payload_gaps_test.go` — NEW
- `apps/local-runner/internal/runner/cp84_mux_stream_test.go` — NEW
  (HTTP-level `handleAllEventsStream`: chunked snapshot + Complete flag,
  resync closes stream, subscriber isolation)

## Verified

- `go test ./internal/runner` — all pass, incl. provider-parity test
  `TestRunUpdates_ClaudeCodexGrokSameProjection` unchanged.

## Provider parity

Verified by existing `TestRunUpdates_ClaudeCodexGrokSameProjection` —
decision payloads are provider-agnostic projections.
