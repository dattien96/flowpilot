# CA-972 — Task-442: real usage-token budget + post-turn ask_user escalation

## Summary

Until now the only token budget was the *estimated* prompt cap — a node could
burn unlimited real tokens across turns. `maxUsageTokens` adds the real
provider-reported cumulative cap per node invocation, enforced **post-turn**
(the running turn always completes; the gate never fires mid-turn) through a
durable ask_user card: `extend` / `rotate` / `stop`.

## What changed

- `internal/agentpack/pack.go` + `flow-pack/flows/task-harness.yaml` —
  `ContextProfile.MaxUsageTokens` (yaml `maxUsageTokens`, 0 = uncapped).
- `internal/runner/context_usage.go` — new file:
  - `nodeUsageTokensLocked` — sums `TokenUsage.Total.TotalTokens` across the
    node's legs (keyed by provider session id); `Last` is never summed.
  - `nodeUsageBudgetFor` — cap + one increment per resolved extend/rotate.
  - `checkUsageBudgetPostTurn` — runs after the turn terminal event;
    appends the est-vs-actual audit line (`prompt-context-audit-<turn>.jsonl`,
    best-effort, never blocks a run) then emits `usage_budget_exceeded` via
    the durable `user_question_required` path — persisted BEFORE emit; on
    persist failure the pending question is rolled back in memory.
  - Options: `extend` always; `rotate` only when `s.usageRouter` (CP-87 seam)
    is wired; `stop` always. Gate unavailable degrades to extend/stop.
  - `applyUsageBudgetAnswer` — extend needs no action (cap derives from the
    durable choice); rotate delegates to `usageRouter.RotateUsageBudgetRun`;
    stop → `applyFlowControl(parent, {status:"escalate"})`.
  - `usageBudgetAutoSelectable` — `extend` is NEVER auto-selectable (it mutates
    the user's declared budget).
  - `appendUsageAuditLine` — est-vs-actual jsonl per turn (calibration input).
- `internal/runner/interactive_service.go` — `questionRecord.kind` +
  `usageBudgetQuestionKind`; `AnswerQuestion` routes engine-generated card
  kinds to semantic handlers instead of an ordinary reprompt; post-turn hook
  invokes the check once per completed turn; `usageRouter` seam field.
- New `task442_usage_budget_test.go` — 12 tests: cumulative accounting per
  node, post-turn-only firing, correct options, rotate delegation,
  gate-unavailable degrade, auto-mode-never-extend, extend-once, stop→
  escalate edge, uncapped/no-profile, provider-silent soft-degrade, audit
  line, Claude/Codex/Grok parity.

## Tests

- `go test ./internal/runner/ -run TestTask442` — 12/12 green.
- Est figure and real usage are reported side-by-side in the audit line —
  never conflated (Task-441 rename semantics carried into the ledger).

## Honest gaps

- `usageRouter` is a seam stub until CP-87 lands the routing gate; the card
  still offers rotate only when the seam is wired.
- Accounting keyed on `providerSessionId` events — a provider that reports
  usage without a session id is not accumulated (degrade-silent, logged).

# ---8<--- flowpilot:change-ledger
feature_key: token-usage
source_doc_id: Task-442
change_type: feature
summary: maxUsageTokens real-usage cap per node — post-turn ask_user extend/rotate/stop via durable question; rotate delegates to CP-87 seam; est-vs-actual audit line per turn; never fires mid-turn
# --->8---
