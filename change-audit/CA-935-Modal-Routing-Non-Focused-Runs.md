# CA-935 — Modal routing: only the focused run raises modals (CP-84 Task-434)

## Summary

Modal-set paths are singletons — a background lane's quota/gate event could
hijack the focused chat. Routing is now explicit:

- `isFocusedRun(state, runId)` — the single decision point; `undefined`/
  unresolvable → false (fail-safe to inbox). `mainRunId` and
  `activeAgentRunId` count as focused (viewing a parent leg or child lane).
- `attentionQueue.ingestAttentionItem(item)` — synthetic items for
  non-focused modal-source events, deduped by (runId, kind); auto-pruned
  once the run's observed status leaves waiting with no actionable decision;
  cleared by `evict`.
- `consumeStream` usage-limit path routes: focused → `pendingAccountSwitch`
  (unchanged); non-focused → synthetic quota `AttentionItem` carrying a
  `quota` `DecisionPayload` with the candidate account id — the inbox's
  "Switch to X" then drives the SAME `pendingAccountSwitch` +
  `confirmAccountSwitch` path as the modal (no forked effect).
- `applyEvent` gateBlock: gated by `isFocusedRun(s, e.workflowRunId)` —
  non-focused blocks still mark `_gateBlockedRunIds` (Navigator stability)
  but surface via the inbox. No-op today (all applyEvent consumers are
  focus-bound) — pure hardening for future non-focused consumers.
- Truly global events (no runId at all) intentionally keep global surfaces.

## Verified

- 8 new tests green (`modalRouting.test.ts`) covering all §7 signatures.
- Focused-run behavior byte-for-byte unchanged; provider-agnostic (routing
  keys on runId only).

## Files

- `state/store.ts`, `state/attentionQueue.ts`,
  `state/modalRouting.test.ts` (new)

# ---8<--- flowpilot:change-ledger
feature_key: attention-queue
source_doc_id: CP-84
change_type: feature
summary: modal routing by source runId — focused runs keep singleton modals, non-focused modal-source events become deduped synthetic inbox items sharing the same confirm paths
# --->8---
