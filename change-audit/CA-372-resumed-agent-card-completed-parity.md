# CA-372: Resumed agent card "— completed" parity with live

## Summary

Fixed BUG-294: on restart, the parent's resumed `AgentTimelineCard` for a child that was still running mid-turn (killed by the restart) rendered a self-contradictory `coder · Claude · cancelled · claude-sonnet — completed`. The `— completed` suffix came from an `EventAgentResultInjected` annotation that `resumedParentAgentAnnotations` (`interactive_resume.go`) emitted purely because the child's persisted `LastMessage` was non-empty, without checking whether the child ever reached a terminal completed state. The LIVE emit path (`interactive_service.go:5162-5171`) only fires that event for `completion.status == RunStatusCompleted` (a failed child errors out before reaching it). The resume path now mirrors that condition exactly: a `completed` flag on the internal `childSession` struct is set from `session.Status == RunStatusCompleted`, and the result-annotation append is gated `if child.completed && child.lastMessage != ""`. Spawn annotations are unchanged, so every child still renders a card — only the false "— completed" suffix is removed for non-completed children.

## Verification

- `go test ./internal/runner -run "TestBug294" -count=1`: 3 passed (2 new dedicated tests + suite).
- `go test ./internal/runner -run "TestBug294|TestRun1264Restore|TestRun1264Settle|TestRun2334RestartReplayKeepsAssistantResponsesForEveryProvider" -count=1`: 12 passed (existing restore/annotation/replay coverage stays green).
- Full `go test ./internal/runner`: residual failures are pre-existing environmental cases (Codex/Gemini CLI binaries, machine home paths); none in the annotation/restore area.
- `-race` not run: this machine lacks gcc/CGO.
- GitNexus impact analysis unavailable on this machine (`%1 is not a valid Win32 application`); performed localized inspection — traced all `EventAgentResultInjected` emit sites (live at `interactive_service.go:5167`, replay at `interactive_resume.go:2030`) to confirm the resume path now matches the live condition.

## Files

- `apps/local-runner/internal/runner/interactive_resume.go`: gate result annotation on `session.Status == RunStatusCompleted`.
- `apps/local-runner/internal/runner/bug294_resumed_agent_card_completed_suffix_test.go`: additive regression tests.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-294
change_type: bugfix
summary: Resume path annotates a child result only when it genuinely completed, matching the live emit condition, so a restart-killed child no longer shows a false "— completed".
# --->8---
