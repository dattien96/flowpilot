---
feature: context-regression-engine
change_type: bugfix
refs: BUG-137, BUG-138, BUG-139, CP-35
---

# CA-126: Fix Gate Block UI Bugs and Correct r-bug Action

## What changed

Three related fixes for the flow-gate UI and rule configuration:

### BUG-137 — Navigator spinner for gate-blocked inactive chats

Added `_gateBlockedRunIds: Record<string, boolean>` to the Zustand store. When a live `flow_gate_violation` with `status === "block"` fires, the run ID is recorded. When `turn_started` fires for that run, the entry is removed. Navigator's `effectiveStatus` for non-active items now checks this set: `gateBlockedRunIds[item.runId] ? "completed" : item.status`.

**Files**: `apps/desktop-flowpilot/src/state/store.ts`, `apps/desktop-flowpilot/src/components/Navigator.tsx`

### BUG-138 — Gate block modal re-pops on every chat open

`consumeOrchestrationStream` was processing `flow_gate_violation` after `_historyReplaying` became false (race with `consumeHistoryReplayStream`). Fixed by adding a dynamic watermark in the non-agent event branch: skip events whose seq is already covered by the history replay (`e.seq <= get()._runReplaySeq[runId] ?? afterSeq`).

**Files**: `apps/desktop-flowpilot/src/state/store.ts`

### BUG-139 — r-bug action should be reprompt, not block

`DefaultRules()` had `r-bug` with `Action: "block"`. This caused a hard-stop modal for a missing bugfix doc instead of an auto-reprompt. Changed to `Action: "reprompt"` to match SD-20 D-3 design intent (same as `r-ca`). Updated SD-20 §2.2 and D-3.

**Files**: `apps/local-runner/internal/flowgate/rules.go`, `requirements/06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md`

## Why

These bugs caused poor UX after a gate block: spinner showing on unrelated chats, modal interrupting the user every time they reopen a blocked chat, and r-bug incorrectly acting as a hard stop instead of prompting the AI to fix its own omission.

## Invariants preserved

- `r-tests` and `r-reg` remain `"block"` — failing tests are never auto-remediable.
- `_historyReplaying` guard for `gateBlock` (no modal on replay) is unchanged.
- CP-35 reprompt path (orchestration stream processes late turn events) is preserved — those events have seqs above the replay watermark.
