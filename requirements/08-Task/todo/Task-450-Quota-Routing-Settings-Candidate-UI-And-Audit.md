# Task-450: Quota Routing Settings, Candidate UI & Audit

- Document ID: `Task-450`
- Title: `Desktop/TUI manual-or-auto settings, candidate table, rotation notices, and requested→resolved audit correlation`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-87`, `Task-446`, `Task-449`, `CP-86`
- Child Documents: ``
- Related Documents: `CP-84 attention queue`, `decision-card-ui`, AI Providers settings
- Replaces: ``
- Tags: `quota`, `ux`, `settings`, `audit`, `desktop`, `tui`

## AI Quick View

### Summary

- Settings expose Manual (default) vs Auto rotation, provider priority,
  class-model bindings, thresholds/cooldown.
- Manual gate shows candidates with provider/model/class/account/headroom/reset/
  confidence and explicit actions; Vibe uses the same gate.
- Auto rotations show non-blocking requested→resolved notice. Audit correlates
  route decision with CP-86 estimated prompt and actual usage.
- Use once/run never mutates Supabase step defaults; only explicit Save as step
  default may do so.

### Current Ask

- Implement accessible Desktop + TUI configuration/decision surfaces and
  durable audit presentation without duplicating routing logic client-side.

### Key Decisions

- `T-1` UI renders runner-provided candidates/reasons/ranking; it never re-ranks.
- `T-2` Manual table columns are contractual: Provider, Model, Workload,
  Account, Headroom, Reset, Confidence, Reason.
- `T-3` Auto-rotation notice is informational; quota gate is actionable and
  enters focused modal or non-focused attention inbox using existing routing.
- `T-4` Settings make the 20s same-provider cooldown and automatic behavior
  clear and warn that automatic rotation never cycles indefinitely. While a
  switch is waiting, Desktop/TUI render a determinate countdown bar (`20s → 0s`)
  plus: "Safety cooldown — rapid switching between multiple accounts of the
  same provider on one IP may trigger provider risk controls." The bar is
  non-blocking for navigation, survives refresh/restart from server timestamps,
  and never restarts from 20s on client remount.

### Constraints

- Accessibility: keyboard table selection, clear focus, no color-only state.
- Missing quota = Unknown, never zero or token count.
- Client does not parse provider errors or mutate run state optimistically.

### Open Questions

- Explicit Save-as-step-default can be deferred if write authority is broad;
  if deferred, hide the action rather than shipping a no-op.

### Source Refs

- `CP-87 P-5/P-7`, Task-446, Task-449, CP-86 audit figures,
  `DecisionControls.tsx`, `AttentionInbox`, `AiProvidersSettings.tsx`.

## 1. Goal

Make quota routing understandable and controllable in Desktop/TUI, with a
complete forensic trail of why execution moved.

## 2. Parent Links

- coding plan: `CP-87 P-6`
- tech design: `SD-07`, `SD-10`
- system spec: `SS-22`
- specific upstream ids: Task-446, Task-449, CP-84

## 3. Trigger

The backend gate/auto engine is unsafe to ship without explicit opt-in,
candidate transparency, and durable audit correlation.

## 4. Exact Change

- `T-1` AI Providers/Engine settings: Manual/Auto, provider order,
  provider+workload model selects, thresholds/cooldown.
- `T-2` Candidate decision table in Desktop and TUI with use-once/use-run/stop;
  optional explicit Save default.
- `T-3` Auto rotation timeline/step-card notice with requested→resolved route.
- `T-4` Audit drawer/record: requested/resolved provider/model/account,
  reason, policy, headroom evidence, est prompt, max usage, actual usage.
- `T-5` Focused/non-focused routing reuses existing decision/attention paths.
- `T-6` Same-provider cooldown renders a determinate progress/countdown bar from
  server `cooldownStartedAt`/`cooldownUntil`, updates at most once per second,
  shows seconds remaining and `same_provider_ip_safety` copy, reaches zero
  without client-side extension, and resumes from the original deadline after
  refresh/restart.

## 5. Touched Areas

- files: Desktop settings/store/decision controls/step card/audit drawer; TUI
  settings picker/decision table/session panel; client DTOs
- modules: Desktop, TUI
- routes: existing settings + decision endpoints
- tables: optional existing step definition update only on explicit action

## 6. Code Guide Signatures

```ts
export type QuotaRotationMode = "manual" | "auto";
export function QuotaRoutingSettings(): JSX.Element;
export function QuotaCandidateTable(props: { decision: QuotaRouteDecision; onSelect(optionId: string): void }): JSX.Element;
export function QuotaRoutingAudit(props: { record: QuotaRoutingAuditRecord }): JSX.Element;
export function SameProviderCooldownBar(props: {
  startedAt: string;
  until: string;
  reason: "same_provider_ip_safety";
  now?: number;
}): JSX.Element;
```

```go
// TUI rendering consumes server candidate order unchanged.
func renderQuotaCandidateTable(decision QuotaRouteDecision, width int) []string
```

## 7. Test Signatures

- `test("quota routing setting defaults Manual")`
- `test("candidate table shows provider model workload account headroom reset confidence")`
- `test("candidate table preserves runner order and reasons")`
- `test("manual selection offers once/run and explicit save-default only when supported")`
- `test("auto rotation notice shows requested and resolved route")`
- `test("unknown headroom is not rendered as zero or tokens")`
- `test("same-provider cooldown bar counts 20 seconds from server timestamps")`
- `test("cooldown bar survives remount without restarting at 20 seconds")`
- `test("cooldown bar explains same-IP provider safety and is not color-only")`
- `TestTask450_TUICandidateTableAndSettings`
- `TestTask450_TUICooldownCountdownUsesServerDeadline`
- `TestTask450_AuditCorrelatesRequestedResolvedAndUsage`

## 8. Acceptance Check

- Operator can configure mode/mappings, resolve a Flow or Vibe quota gate from
  Desktop/TUI, and inspect the exact routing/usage audit after completion.
- For a same-provider rotation, both clients show the same 20s determinate bar,
  seconds remaining, and same-IP safety explanation; refresh/restart resumes
  from the server deadline rather than resetting the wait.

## 9. Out of Scope

- Client-side ranking/error parsing.
- Silent base step model mutation.

## 10. Definition of Done

- [ ] §6 signatures landed or deviation documented
- [ ] §7 additive tests green
- [ ] Desktop + TUI manual/auto/candidate/audit UX verified
- [ ] Missing data labels honest; accessibility pass
- [ ] Save-default explicit or cleanly omitted
- [ ] CA ledger + feature key entries complete
- [ ] GitNexus detect_changes reviewed before commit

## 11. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
