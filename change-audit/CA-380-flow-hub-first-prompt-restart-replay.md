# CA-380 — Flow-hub first prompt restart replay fix

## Summary

Fixed BUG-300: a flow-hub chat run's original first prompt could vanish from
the transcript after a server restart when the hub's own turn 1 spawns its
entry child directly and never itself makes a real provider call (its first
genuine Claude/Codex turn is the round-0 synthesis turn). `overlayRawTurnPrompts`
had no replayed `turn_started` event to overlay the durable raw prompt onto in
that case, so the prompt was silently dropped instead of rendered.

`seedTranscriptFromDisk` (shared Claude/Codex restart-replay path) now calls
`prependMissingPromptOnlyEvents` right after `overlayRawTurnPrompts`, the same
safety net `seedGrokTranscriptFromDisk` already had. Any raw prompt from the
durable turn log that isn't matched onto a replayed provider turn is restored
as its own synthetic prompt-only turn.

## Cross-provider parity

Classification: shared replay contract fix, single call site
(`seedTranscriptFromDisk` in `interactive_resume.go`), applies to both Claude
and Codex (the two providers that share this function). Grok already had the
equivalent guard; this closes the gap rather than duplicating a third
implementation.

## Verification

- `go test ./internal/runner -run 'TestClaudeRestartRestoresFirstPromptWhenHubNeverCalledProviderOnTurnOne'` — new regression test, confirmed to fail without the fix (git-stash verified) and pass with it.
- `go test ./internal/runner -run 'TestRun1264|TestRun2334|TestRun5695|TestRun20332|Restart|Resume|Replay|Transcript'` — 191 passed before and after the fix; the 8 failures present (Codex CLI-binary-dependent tests, one unrelated pre-existing failure) reproduce identically either way.
- `go build ./...` — passed.

## Root cause vs symptom

The turn 1 → direct-child-spawn pattern is intentional flow-hub behavior, not
itself a bug — the bug was that the restart-replay path assumed every prompt
in the durable turn log would always find a matching provider-transcript
`turn_started` event to attach to. That assumption silently broke exactly when
a turn produces no provider call at all, dropping user-facing history.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-300
change_type: bugfix
summary: Restore a flow-hub's original first prompt on restart replay when its turn 1 never itself calls the provider (spawns the entry child directly instead), mirroring the existing Grok safety net for Claude/Codex.
# --->8---
