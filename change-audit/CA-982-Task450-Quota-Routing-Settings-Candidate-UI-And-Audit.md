# CA-982 — Task-450: Quota routing settings, candidate UI & audit

## Summary

The quota gate now has a face. `quota_route_required` cards carry the
structured `QuotaRouteDecision` (runner-ordered candidates with headroom,
reset, confidence, rejection reasons, and same-provider cooldown stamps)
end-to-end — durable question record → restart rehydration → snapshot →
decision projection → Desktop `QuestionCard` and TUI event/snapshot paths.
Clients render the runner's order and reasons verbatim: no client-side
ranking, no provider-error parsing, no optimistic run-state mutation.
`GET /client/workflow-runs/{id}/quota-audit` correlates each committed
route with the policy version, headroom evidence, and CP-86 usage figures
(estimated prompt, node max usage, actual usage).

## What changed

- `runner/quota_decision.go` (new) — `QuotaRouteDecision` /
  `QuotaRouteCandidate` wire DTOs, `quotaDecisionForRecord`, and the
  `QuotaRoutingAuditRecord` assembler walking the durable
  quota-route/question/event records.
- `runner/workflow_store.go` — `ProviderQuestionState.QuotaDecision`
  persists the structured payload so a pending quota card rehydrates
  byte-identical after restart.
- `runner/interactive_service.go` — `questionRecord.quotaDecision`,
  persisted state round-trip, and the three rehydration sites restore it
  into the record.
- `runner/decision_payload.go` — `questionDecisionPayload` emits `quota`
  on decision payloads for quota-kind cards (additive field; CRITICAL
  impact flagged pre-edit — populated only for quota decisions).
- `runner/interactive_handlers.go` — `pendingQuestionView.QuotaDecision`
  on the snapshot view; `GET .../quota-audit` route + handler.
- `runner/quota_gate.go` — the emitted card carries the structured
  decision; committed/stopped/blocked route payloads gained
  `policyVersion`, `headroom`, and cooldown stamps.
- `runner/provider_event.go` — `QuotaRoutePayload` audit fields.
- TUI `client/client.go` — `QuotaRouteDecision`, `QuotaRouteCandidate`,
  `QuotaRoutePayload`, `QuotaRoutingAuditRecord`, `QuotaRoutingSettings`
  mirrors + `GetQuotaAudit`/`GetQuotaRoutingSettings`/
  `SetQuotaRoutingSettings`.
- TUI `quota_table.go` (new) — `renderQuotaCandidateTable` (contractual
  columns, runner order verbatim), `renderQuotaCooldownBar` (determinate
  `20s → 0s` from server stamps), `renderQuotaRoutingSettings`,
  `renderQuotaAudit`.
- TUI `app.go`/`model.go` — `QuestionState.Quota`, committed/stopped/
  blocked route notices, `QuotaSettingsMsg`/`QuotaAuditMsg`, and the
  `/quota` + `/quota audit` commands.
- TUI `history.go` — pending-question snapshot restores `QuotaDecision`.
- Desktop `contract.ts` — `QuotaHeadroomDTO`, `QuotaRouteCandidateDTO`,
  `QuotaRouteDecisionDTO`, `QuotaRouteDTO`, `QuotaRoutingAuditRecord`,
  the three `quota_route_*` event variants, `quotaDecision` on the
  question event, and the optional client surface
  (`getQuotaRoutingSettings`/`setQuotaRoutingSettings`/
  `getQuotaRoutingAudit`).
- Desktop `timelineReducer.ts` — `quotaDecision` rides question items and
  `pendingQuestions` (conditionally spread — an always-set `undefined`
  key broke a `deepStrictEqual` on the pendingQuestions shape);
  `quota_route_*` events emit informational system lines with the
  requested→resolved route.
- Desktop `QuestionCard.tsx`/`Timeline.tsx` — a quota card renders
  `QuotaCandidateTable` instead of the flat option list.
- Desktop `components/quota/` (new) — `QuotaCandidateTable` (Once/For-run/
  Stop, disabled-with-reason rows, cooldown bar per cooling row),
  `SameProviderCooldownBar` (ARIA progressbar, ≤1 Hz tick, text+bar so
  state is never color-only), `QuotaRoutingAudit` (dl record).
- Desktop `settings/QuotaRoutingSettings.tsx` (new) + `EngineSettings.tsx`
  — Manual/Auto mode, provider priority, class model bindings, headroom
  threshold, telemetry TTL, same-provider cooldown; mounted as an Engine
  subpanel, refreshed from the runner on mount.
- Desktop `styles.css` — `.quota-*` styles.

## Decisions

- `quotaDecision` is conditionally spread into question items —
  `deepStrictEqual` treats `{k: undefined}` ≠ missing key, and an
  always-set field changed the pendingQuestions contract for plain
  questions (caught by `user_question_required adds question card …`).
- Save-as-step-default is hidden, not a shipped no-op (spec's open
  question): the UI offers Once/For-run/Stop only; Supabase step defaults
  are never mutated from this surface.
- TUI `/quota` is read-only parity: the runner owns the settings
  document, Desktop owns editing (mirrors `/provider` parity). `/quota`
  renders mode/priority/bindings/thresholds/cooldown; `/quota audit`
  renders the forensic record.
- Cooldown math derives `total` from `until − startedAt` (not a hardcoded
  20) and `remaining` from `until − now` — remount/refresh resumes from
  the server deadline; elapsed clamps at 0, never negative.
- Missing headroom renders the state word (`unknown`/`stale`), never a
  fabricated `%` or token count.

## Verification

- `go test -count=1 -run 'TestTask450' ./internal/runner/` — green
  (structured card payload, audit correlation, restart rehydration).
- `go test -count=1 -run 'TestTask450' ./internal/tui/app/` — green
  (candidate table + settings view; cooldown countdown/remount/zero).
- Desktop `node --test` on
  `components/quota/QuotaComponents.render.test.js` (9 tests: columns,
  order, reasons, once/run/stop, no save-default, notice route, honest
  unknown, 20s countdown, remount resume, non-color-only, audit
  correlation), `state/quotaRouting.test.js` (incl. `quota routing
  setting defaults Manual`), `state/providerLimit.test.js` — 14/14
  green; `state/timelineReducer.test.js` — green after the conditional
  spread fix.
- `npx tsc --noEmit` — clean.
- Full desktop phase1 suite: 521/531 — the 10 failures were the
  documented pre-existing set (history-replay ordering ×3, selectProject,
  openHistoryRun, replay-abort, handoff settle, stop semantics,
  terminal-replay Thinking) plus the one Task-450 casualty listed above,
  which is fixed.
- **Cross-provider parity** (per `.agents/skills/cross-provider-parity`):
  Case 1 — provider-agnostic. Decision payloads, candidate tables,
  cooldown bars, and the audit record carry `providerKey` as data; no
  client or renderer branches on a provider constant. Candidate
  enumeration stays registry-driven (Task-448).
- **Durable replay** (per `.agents/skills/durable-replay-contracts`):
  interactive-state row — the quota card's structured payload persists on
  `ProviderQuestionState`, rehydrates via the question-record restore
  seam, and reaches the projection/snapshot view identically
  (`TestTask450_RehydratedCardRestoresQuotaDecision`); route notices are
  informational timeline rows emitted once at commit (no replay
  duplication — the committed-rotation non-replay invariant from
  Task-449 holds).
- detect_changes: GitNexus CLI exposes no `detect_changes` (MCP-only,
  not configured); equivalent staged-diff scope review performed —
  staged set matches the declared Task-450 file list.

## Follow-ups

- Desktop live smoke of a real `quota_route_required` card against a
  connected runner (fixture tests cover render/store paths).
- Save-as-step-default if the open question resolves to shipping it.
