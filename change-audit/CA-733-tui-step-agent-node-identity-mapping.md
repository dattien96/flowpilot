# CA-733 — TUI step/agent highlight pinned by node identity, not shared catalog agent (task-harness S3)

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-58
change_type: bugfix
summary: childRunForStep now pins a flow step to its spawned child by node identity (Label == NodeID) first, and only falls back to the shared catalog agent name for unlabeled runs — so two nodes that share one agent (task-harness plan_writer+implement on coder, plan_reviewer+reviewer on reviewer) no longer highlight/agent-chip the wrong step when a child is focused
# --->8---

## Problem

- Live task-harness S3 (run-198924): with `plan_writer` and `implement` both spawning from `agents/coder.md` (and `plan_reviewer`/`reviewer` both from `agents/reviewer.md`), `childRunForStep` matched a step to a child by any of {AgentRef, NodeID, StepType} against the child's {RunID, AgentName, Label}.
- The plan_writer child (`AgentName="coder"`, `Label="plan_writer"`) therefore matched BOTH the `plan_writer` row AND the still-PENDING `implement` row (`AgentRef="agents/coder.md"`). The PENDING implement row gained a phantom `· agent: coder` chip, and `/agents plan_writer` (focus on the plan_writer child) highlighted `implement` instead of `plan_writer`.
- `Now: plan_writer` on the status line was correct — it comes from `activeStepName(msg.Steps)`, which does not use this mapper.

## Changes

- `apps/local-runner/internal/tui/app/agents_focus.go` (`childRunForStep`): matching is now step-identity-first:
  1. A child with a non-empty `Label` matches only `NodeID`/`StepType` (spawnChildRun sets `Label = node.ID`, so a labeled child is pinned to its own node);
  2. an unlabeled child falls back to `RunID`/`AgentName` against the step's node identity, then the shared `AgentRef` catalog name as a last resort.
  The focused-run-first pass (CA-529) and first-match fallback (CA-513/524) are preserved.

## Tests added (new file only)

- `task_harness_step_agent_mapping_test.go` (provider matrix Claude/Codex/Grok):
  - same-catalog pair `plan_writer`/`implement` (one child `Label=plan_writer`): plan_writer maps, implement does not;
  - same pair for `plan_reviewer`/`reviewer`;
  - steps panel: focusing plan_writer marks only its row; implement shows no marker and no agent chip;
  - near-miss: unlabeled child (`AgentName=coder`, node `coder`) still maps by agent name (rag-harness shape);
  - focused-wins (CA-529) and first-match (CA-513/524) regression guards retained.

## Verification

- `go test ./internal/tui/app/ -count=1` full package PASS (old TUI suite untouched and green).
- `go vet ./internal/runner/ ./internal/tui/app/ ./internal/agentpack/` clean.
- Provider-agnostic: rendering logic; matrix covers all three providers.