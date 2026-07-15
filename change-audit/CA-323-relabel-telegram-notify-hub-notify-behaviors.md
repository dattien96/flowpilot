# CA-323: Relabel telegram.notify / hub.notify Dropdown Options For Clarity

## Scope

Fixes BUG-283: the two Telegram-notify node behaviors introduced by Task-235/Task-236 (`telegram.notify` static/no-AI, `hub.notify` AI-composed) had dropdown labels reading as near-synonyms ("(no agent)" vs "(no child agent)"), which led a user to pick `telegram.notify` while expecting AI composition — confirmed via `run-8059`'s diagnostic log (`flow_telegram_sent` event, not `hub_notify_reinvoke_scheduled`).

## Changes

- `packages/flowpilot-client-core/src/domain/adminModels.ts`: reworded the two `FLOW_BEHAVIOR_OPTIONS` labels so "STATIC ... no AI" and "AI-COMPOSED" are the first distinguishing words:
  - `telegram.notify`: `"Telegram notify — send a Telegram message (no agent)"` → `"Telegram notify — STATIC message, no AI (fixed text / template sent verbatim)"`
  - `hub.notify`: `"Hub notify — main agent composes & sends a notification (no child agent)"` → `"Telegram notify — AI-COMPOSED message (main agent's own turn, no child agent)"`
- No `behaviorId` string values changed (no data migration needed); no dispatch/registry code touched.

## Verification

- `npx tsc --noEmit` (apps/desktop-flowpilot) — clean.
- Not independently re-tested against a live desktop session (no running app available this pass) — a string-literal label change with no branching logic, so `tsc` plus manual side-by-side re-read of the two labels was judged sufficient.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via direct inspection; low risk since only a string literal in a data array changed, no symbol logic touched.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-283
change_type: bugfix
summary: reword the telegram.notify/hub.notify Behavior ID dropdown labels so static-vs-AI-composed is unmistakable, after ambiguous wording led a user to configure the wrong node
# --->8---
