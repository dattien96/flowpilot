# CA-362 — run-1264 settle attention and restore hygiene

## Summary

Fixed two regressions observed on `run-1264`:

- completed flow turns with stale `pendingFlowGateSettle` now allow durable settle finalization when the parent loop is already `done`, removing the residual `settle_pending` operator attention card;
- flow-engine internal joined-note prompts are no longer treated as history titles, and transcript replay keeps internal joined-note prompts out of the user-facing transcript across providers while restoring the original raw user prompt when the flow-start turn had no provider session.
- resume now keeps provider transcript timestamps and, when a provider transcript lacks them, replaces durable `spawn_agent` tool anchors in-place or places direct flow-engine child cards immediately before synthesis; result events update those cards without creating bottom-of-timeline rows.

## Files

- `apps/local-runner/internal/runner/dispatch_settle_wire.go`
- `apps/local-runner/internal/runner/bug083_test.go`
- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/interactive_resume.go`
- `apps/local-runner/internal/runner/feature_history.go`
- `apps/local-runner/internal/runner/grok_transcript_loader.go`
- `apps/local-runner/internal/runner/transcript_loader.go`
- `apps/local-runner/internal/runner/run1264_settle_and_restore_test.go`
- `apps/desktop-flowpilot/src/components/Timeline.tsx`
- `apps/desktop-flowpilot/src/state/timelineReducer.ts`
- `apps/desktop-flowpilot/src/state/timeline_agent_lifecycle.test.ts`
- `apps/desktop-flowpilot/src/styles.css`
- `apps/desktop-flowpilot/src/types/contract.ts`

## Verification

- Added regression tests for stale settle finalization, provider-neutral restore prompt filtering, timestamped child lifecycle ordering, and the desktop agent-card reducer path.
- Updated the prior Codex prompt-only fallback test after explicit approval because its expected replay of `[flow-engine] Agent results ready.` conflicted with the provider-agnostic user-facing transcript spec.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Restore agent lifecycle events at durable timestamps or spawn-tool anchors as completed cards without replaying internal orchestration prose; flow-engine cards without provider tool anchors are inserted before synthesis.
# --->8---
