---
feature: context-regression-engine
change_type: bugfix
refs: BUG-137, BUG-138, BUG-139, BUG-140, CP-35
---

# CA-126: Fix Gate Block UI Bugs and Correct r-bug Action

## What changed

Three related fixes for the flow-gate UI and rule configuration:

### BUG-137 — Navigator spinner for gate-blocked chats

Added `_gateBlockedRunIds: Record<string, boolean>` to the Zustand store. When a live `flow_gate_violation` with `status === "block"` fires, the run ID is recorded; cleared on the next `turn_started`. Navigator's `effectiveStatus` now treats a gate-blocked run as `"completed"` **regardless of active state**: `gateBlockedRunIds[item.runId] ? "completed" : (isActive ? status : item.status)`. The active-chat case matters because `openHistoryRun` sets the live `status` from the resumed handle (`"running"`), so a reopened blocked chat would otherwise show a stuck spinner even while active.

**Files**: `apps/desktop-flowpilot/src/state/store.ts`, `apps/desktop-flowpilot/src/components/Navigator.tsx`

### BUG-138 — Gate block modal re-pops on every chat open

`consumeOrchestrationStream` was processing `flow_gate_violation` after `_historyReplaying` became false (race with `consumeHistoryReplayStream`). The first pass added a dynamic watermark in the non-agent event branch (`e.seq <= get()._runReplaySeq[runId] ?? afterSeq`), but that still relied on stream timing. The definitive fix makes `_gateBlockedRunIds` the authoritative, race-free modal guard: `applyEvent`'s `flow_gate_violation` block branch only sets `gateBlock` when the run is **not already** in `_gateBlockedRunIds` (added on the first block, cleared on the next `turn_started`). Reopening a blocked chat re-streams the event but finds the run already flagged → no re-pop.

**Files**: `apps/desktop-flowpilot/src/state/store.ts`

### BUG-139 — r-bug action should be reprompt, not block

`DefaultRules()` had `r-bug` with `Action: "block"`. This caused a hard-stop modal for a missing bugfix doc instead of an auto-reprompt. Changed to `Action: "reprompt"` to match SD-20 D-3 design intent (same as `r-ca`). Updated SD-20 §2.2 and D-3.

**Files**: `apps/local-runner/internal/flowgate/rules.go`, `requirements/06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md`

### BUG-140 — Reprompt lacked actionable remediation

Once `r-bug` became a `reprompt` (BUG-139), the gate reprompted the AI with `result.Message` — the terse symptom (`"Flow gate: bug fix detected but no bugfix doc found"`). The AI could not self-correct (in E2E-11 it edited the change-audit note instead of creating a BUG doc). Added `flowgate.RepromptPrompt(EnforceResult)` which builds per-rule, file-level instructions (`requirements/09-BugFix/done/BUG-<NNN>.md` for `r-bug`, `change-audit/CA-<NNN>.md` for `r-ca`, plus an explicit "do NOT edit the change-audit note" guard). `gate_hook.go` sends this as the reprompt prompt; the inline desktop card still uses the terse `result.Message`.

**Files**: `apps/local-runner/internal/flowgate/enforce.go`, `apps/local-runner/internal/runner/gate_hook.go`, `apps/local-runner/internal/flowgate/flowgate_test.go`

## Why

These bugs caused poor UX after a gate block: spinner showing on unrelated chats, modal interrupting the user every time they reopen a blocked chat, and r-bug incorrectly acting as a hard stop instead of prompting the AI to fix its own omission.

## Invariants preserved

- `r-tests` and `r-reg` remain `"block"` — failing tests are never auto-remediable.
- The block modal still fires exactly once on the live block: the first `flow_gate_violation` finds the run absent from `_gateBlockedRunIds`, sets `gateBlock`, then flags the run. Subsequent re-streams find it flagged → no re-pop. The flag clears on `turn_started` so a genuinely new block surfaces again.
- CP-35 reprompt path (orchestration stream processes late turn events) is preserved — those events have seqs above the replay watermark, and the reprompt prompt is now actionable (BUG-140).
- `Enforce` resolution/severity is unchanged; `RepromptPrompt` is an additive helper used only for the AI-facing prompt.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: BUG-137
change_type: fix
summary: Fix Gate Block UI Bugs and Correct r-bug Action
# --->8---
