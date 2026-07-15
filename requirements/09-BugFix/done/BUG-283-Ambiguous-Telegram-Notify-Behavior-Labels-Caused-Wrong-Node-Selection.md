# BUG-283: Ambiguous `telegram.notify`/`hub.notify` Labels Caused User To Pick The Wrong Node

## Metadata

- Document ID: `BUG-283`
- Title: `Ambiguous telegram.notify/hub.notify Behavior Labels Caused User To Pick The Wrong Node`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [Task-236: telegram.notify — Deterministic (No-Agent) Telegram Send Node Behavior](../../08-Task/done/Task-236-Telegram-Notify-Deterministic-Inline-Node-Behavior.md), [Task-235: Hub Notify Node Behavior](../../08-Task/done/Task-235-Hub-Notify-Node-Behavior-Reinvoke-Hub-For-AI-Composed-Notification.md)
- Child Documents: `None`
- Related Documents: [Task-237: Generalize Post-Node "Done" Edge-Walking](../../08-Task/done/Task-237-Generalize-Post-Node-Done-Edge-Walking-Audit-And-Hub-Inline-Successor-Chaining.md), run diagnostic log `.flowpilot/logs/features/agent-flow-engine/run-8059.ndjson` (evidence)
- Replaces: `None`
- Tags: `agent-flow-engine, node-behavior, telegram, ui, labeling, ux`

## AI Quick View

### Summary

- Right after `Task-235`/`Task-236` shipped both `telegram.notify` (static, no AI) and `hub.notify` (AI-composed, via the hub's own turn), the user built a flow expecting an AI-composed Telegram message but the node still sent the static/template text.
- Root cause confirmed via `run-8059`'s diagnostic log: the node (`tele-step`) had `behaviorId="telegram.notify"`, not `"hub.notify"` — the two `FLOW_BEHAVIOR_OPTIONS` dropdown labels at the time ("send a Telegram message (no agent)" vs "main agent composes & sends a notification (no child agent)") read as near-synonyms differing only in "(no agent)" vs "(no child agent)", not as opposites.
- Not a runtime defect — the flow engine dispatched exactly what was configured (`flow_telegram_sent` event, Go-only deterministic path) and `Task-237`'s edge-walking / `Task-235`'s hub reinvoke both worked correctly when actually selected. The bug is the UI copy leading a user to configure the wrong node.

### Current Ask

- Reword both dropdown labels so "static, no AI" and "AI-composed" are unmistakable at a glance, with no other behavior change.

### Key Decisions

- `V-1` Fix is UI-copy-only: keep both `behaviorId` string values (`telegram.notify`, `hub.notify`) unchanged so no existing `step_definitions` row or authored flow needs migrating — only the `label` shown in the Behavior ID dropdown changes.
- `V-2` Do not delete `telegram.notify` — it has a legitimate distinct use case (zero AI/token cost, deterministic, Go-verified `message_id`) that `hub.notify` cannot substitute for a fixed/no-compose message.

### Constraints

- No data migration; no behavior-dispatch code touched (`behavior_registry.go`, dispatch switches in `flow_validate_audit_dispatch.go`/`interactive_service.go` are unchanged).
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via direct inspection of the diagnostic log and the UI source instead of automated impact analysis (low risk here: a string-literal label change touches no symbol logic).

### Open Questions

- `Q-1` Whether an existing, already-saved step still shows the OLD label text anywhere it was cached client-side (e.g. a stale in-memory list) until the desktop app reloads step definitions — not verified in this pass; low risk since the dropdown always renders from the current `FLOW_BEHAVIOR_OPTIONS` constant, not a cached label string.

### Source Refs

- `run-8059` diagnostic log (`.flowpilot/logs/features/agent-flow-engine/run-8059.ndjson`): line with `"event":"flow_telegram_sent"`, `"node_id":"tele-step"` — the direct evidence this node's behavior was `telegram.notify`, not `hub.notify`.
- `Task-235`, `Task-236` (the two behaviors whose labels this bug corrects).

## 1. Issue Summary

The user reported: "vẫn không nhận real mes trên tele (vẫn là templated mes receiver)" — after `hub.notify` shipped, a flow node still sent the static/template message instead of an AI-composed one. Inspecting the actual run's diagnostic log showed the node's behavior was still `telegram.notify` (the pre-existing, Go-only, no-AI behavior), not `hub.notify` (the new AI-composed one) — the user had picked the wrong dropdown option because the two labels read as near-synonyms.

## 2. Parent Links

- impacted coding plan: none directly — this is a UI-copy correction on top of `CP-42`'s node-behavior-authoring UI, not a plan change
- impacted tech design: none — no `SD` describes exact dropdown copy
- impacted system spec: none

## 3. Environment and Reproduction

- environment: Desktop app → Settings → Workflows → Steps, `context-coding-review-synthesis`-shaped flow with a Telegram notify step named `tele-step`, bound to a `telegram.v1` OUTPUT artifact.
- reproduction steps:
  1. Open the step's Behavior ID dropdown (as it read before this fix): `"Telegram notify — send a Telegram message (no agent)"` vs `"Hub notify — main agent composes & sends a notification (no child agent)"`.
  2. Pick the first option expecting it to mean "the flow's own agent sends it" (a reasonable reading, since neither label says "static" or "AI-composed").
  3. Run the flow. The step sends the artifact's fixed Message Template verbatim (or the default `"[FlowPilot] Run finished..."` text) instead of an AI-composed message.
- frequency: deterministic — any user picking the pre-fix `telegram.notify` label while expecting AI composition reproduces this every time, since the two behaviors are functionally correct for what they were actually configured to do.

## 4. Expected vs Actual

- expected: the dropdown labels make it obvious, without reading source code or a Task doc, which option produces a static message and which one lets the AI compose it.
- actual (confirmed via `run-8059`): the user selected `telegram.notify` believing it would produce an AI-composed message; the flow correctly sent the static/template text for that behavior, which read to the user as "still broken" until the run's diagnostic log was inspected.

## 5. Impact

- users affected: anyone authoring a flow with either Telegram notify behavior via the desktop Settings UI.
- workflows affected: any flow with a Telegram OUTPUT notify step — the confusion is specific to the two-option choice introduced by `Task-235`/`Task-236` landing together.
- severity: low — no data corruption or runtime defect; purely a UX/copy issue that costs a debugging round-trip (as it did here) but is trivially fixed once diagnosed.

## 6. Root Cause

- hypothesis: the node's configured behavior did not match what the user intended to select.
- confirmed cause: `run-8059`'s diagnostic log line `{"event":"flow_telegram_sent","node_id":"tele-step","chat_id":"-5139216327","message_id":"11"}` is emitted ONLY by `runTelegramNotifyNode` (`telegram.notify`'s Go-only dispatch, `flow_validate_audit_dispatch.go`) — never by the `hub.notify` path, which would instead log `hub_notify_reinvoke_scheduled`. No such event appears anywhere in the run. The node was therefore configured with `behaviorId="telegram.notify"`, not `"hub.notify"`, at authoring time — a user-selection outcome, not a dispatch bug — driven by the two `FLOW_BEHAVIOR_OPTIONS` labels at the time reading as near-synonyms ("(no agent)" vs "(no child agent)") rather than as opposites ("static" vs "AI-composed").
- evidence: the same log's earlier lines show the flow otherwise behaving exactly per `Task-235`/`Task-237`'s design — `hub_reinvoke_scheduled` (synthesis's own cohort-join reinvoke) at `10:13:43.557`, then `flow_telegram_sent` for `tele-step` at `10:13:53.920` (~10s later, inside that same hub turn's completion handling via `advanceHubDoneThroughEdge`), then the internal terminal-edge settle (`flow_control_received`/`flow_control_done`) at `10:13:53.928`–`.949`. The engine dispatched exactly what was configured; only the configuration itself was the wrong choice.

## 7. Fix Strategy

- `F-1` Reword `FLOW_BEHAVIOR_OPTIONS`'s two Telegram-notify entries (`packages/flowpilot-client-core/src/domain/adminModels.ts`) so the static/AI-composed distinction is the FIRST thing read, not a trailing parenthetical:
  - `telegram.notify` → `"Telegram notify — STATIC message, no AI (fixed text / template sent verbatim)"`
  - `hub.notify` → `"Telegram notify — AI-COMPOSED message (main agent's own turn, no child agent)"`
- `F-2` (rejected) Deleting `telegram.notify` entirely — rejected because it has a legitimate distinct use case (zero AI/token cost, deterministic, Go-verified `message_id`) that a relabel preserves without losing capability.
- `F-3` (rejected) Renaming the underlying `behaviorId` strings (e.g. `telegram.notify` → `telegram.notify.static`) — rejected: would require a data migration for every already-saved `step_definitions` row and authored flow using either id; a label-only fix has zero migration cost and fully resolves the observed confusion.

## 8. Validation

- `V-1` `npx tsc --noEmit` (apps/desktop-flowpilot) — clean; the relabel is a string-literal change with no type impact.
- `V-2` Manual re-read of both new label strings side by side confirms "STATIC ... no AI" and "AI-COMPOSED" are the first distinguishing words in each, unlike the pre-fix wording — not independently user-tested in this pass (no live desktop session available to re-run the exact confusion scenario).

## 9. Regression Guard

- tests: none added — this is a UI string change with no branching logic to regress; existing `flow_hub_notify_test.go` / `flow_telegram_notify_test.go` (`Task-235`/`Task-236`) already cover the two behaviors' actual dispatch correctness, which this bug never disputed.
- alerts: none applicable.
- audit checks: `change-audit/CA-323` records this fix; `gitnexus_detect_changes()` not run (GitNexus unavailable this session) — manually confirmed only `adminModels.ts`'s `FLOW_BEHAVIOR_OPTIONS` array was touched.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — `Task-235`/`Task-236` already document the correct underlying behavior; only the desktop-facing label text (not covered by any upstream `SS`/`SD`/`CP`) needed correcting.
- notes left unchanged on purpose: the `behaviorId` values (`telegram.notify`, `hub.notify`) and their `registry.yaml` doc entries stay as-is — only the desktop dropdown's display text changed.
