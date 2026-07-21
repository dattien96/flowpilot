# CA-371: Live turn_started hides internal system prompts

## Summary

Redacted the DISPLAY prompt on the live `turn_started` event for internal flow-engine / system prompts (joined result note, gate reprompt, cross-provider handoff), so the live transcript matches the resume/replay transcript — which already hides them via `userFacingTranscriptEvents` → `isInternalTranscriptEvent`. Before this, such a prompt appeared as a chat bubble live but vanished after a server restart + reopen, which a user observed as "one message lost" in a review-loop run (BUG-293 / CP-51 A1). The provider still receives the full prompt: `in.Prompt` continues to flow to `runTurn` and the turn log unchanged; only the display copy on the emitted event is redacted. New pure helper `liveTurnStartedDisplayPrompt` is co-located with its replay twin `isInternalTranscriptEvent` and keyed on the same `isSystemPrompt` predicate so the two paths cannot drift.

## Verification

- `go test ./internal/runner -run "TestBug293" -count=1`: 9 passed.
- `go test ./internal/runner -run "TestBug293|TestRun2334|TestHubNotify|TestRun9437|TestRun1618|TestHubParked|Reprompt|Transcript|FeatureBucket|JoinedNote|Synthesis|Reinvoke|Handoff" -count=1`: 107 passed.
- Full `go test ./internal/runner`: remaining failures are pre-existing environmental cases (Codex/Gemini CLI binaries, machine-specific provider home paths, git-guard shim, Google Drive provider config, skill filesystem merges); none in the prompt/transcript/hub area.
- `-race` not run: this machine lacks gcc/CGO.
- GitNexus impact analysis unavailable on this machine (`%1 is not a valid Win32 application`); performed localized inspection — the only live `turn_started`-with-prompt emit site is `interactive_service.go:6863`; all other `EventTurnStarted{Prompt}` sites are replay loaders already filtered by `userFacingTranscriptEvents`.

## Files

- `apps/local-runner/internal/runner/interactive_resume.go`: add `liveTurnStartedDisplayPrompt`.
- `apps/local-runner/internal/runner/interactive_service.go`: use it at the live `turn_started` emit.
- `apps/local-runner/internal/runner/bug293_joined_note_live_replay_symmetry_test.go`: additive regression tests.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-293
change_type: bugfix
summary: Redact internal system prompts from the live turn_started bubble so the live transcript matches replay, fixing the joined-result-note "lost message" after restart.
# --->8---
