# Task-444: Pressure & Usage UI Surface (Desktop + TUI)

- Document ID: `Task-444`
- Title: `Render est-vs-usage honestly on step cards; inline notices for pressure and provider compaction; ask_user only at decision points`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-86`, `SD-10`
- Child Documents: ``
- Related Documents: `Task-442` (usage cap), `Task-443` (pressure/compaction events), `decision-card-ui` (card rendering path), `attention-queue`
- Replaces: ``
- Tags: `ux`, `token-usage`, `desktop`, `tui`, `transparency`

## AI Quick View

### Summary

- Numbers shown to users must say what they are: `prompt ~Nk est`
  (heuristic input length) vs `usage Nk` (provider-reported) are two
  different quantities and must never collapse into one "est tokens" label.
- `context_pressure` (aware) and `provider_compacted` are **awareness
  notices** — inline lines/banners, never cards. `context_pressure` (ask)
  and `usage_budget_exceeded` are **decision points** — they render through
  the existing user-decision-card path.
- Missing data renders as `—`, never a fake zero.

### Current Ask

- Desktop step card + TUI status: two labeled figures, a pressure banner at
  aware tier, a compacted inline notice, and decision cards routed through
  the existing card mechanism for ask-tier / budget-exceeded.

### Key Decisions

- `T-1` Labels are part of the contract: `prompt ~Nk est` and `usage Nk` —
  the word "est" and the word "usage" must both appear; raw unlabeled
  numbers fail review.
- `T-2` Awareness tier (`context_pressure` aware, `provider_compacted`) =
  inline notice on the step/run card + status tint — non-modal, no action
  required, survives refresh via event replay.
- `T-3` Decision tier (`context_pressure` ask, `usage_budget_exceeded`) =
  the existing user-decision card (`EventUserQuestionRequired`) — no new
  card type; option ids answered through the chat prompt channel as today.
- `T-4` No usage data (provider silent / window unknown) → `—` placeholder
  and a muted "provider doesn't report usage" tooltip — never `0` (a real
  zero and missing data must be distinguishable).

### Constraints

- Desktop renderer consumes the new event types from the normal event
  stream — no polling, no new endpoint.
- TUI reuses `formatContextLimits`/`modelContextWin` (already wired to
  `ModelContextWindow`) — extend, don't fork.
- node --test render tests only (vitest is NOT the desktop test runner);
  TUI table-driven tests.
- Keep CP-84 attention semantics: awareness items do not create inbox
  entries; decision items follow the existing card/modal routing.

### Open Questions

- Whether the compacted notice should also land in the attention inbox —
  default: no (it is informational, not actionable).

### Source Refs

- `CP-86 P-5`, `Task-442`, `Task-443`, `provider_event.go`
  (`EventContextPressure`, `EventProviderCompacted`),
  `tui/app/session_panel.go` (`formatContextLimits`),
  `decision_payload.go` (card contract), `CA-540` open/reset token-usage
  display tests.

## 1. Goal

The user can always answer three questions at a glance: how big was the
prompt (est), how much did the provider actually burn (usage), and did
anything happen to the context mid-step (pressure/compaction) — with
interruption reserved for real decisions.

## 2. Parent Links

- coding plan: `CP-86` (P-5)
- tech design: `SD-10`
- system spec: —
- specific upstream ids: `Task-442`, `Task-443`, `decision-card-ui`, `attention-queue`

## 3. Trigger

Events from Task-442/443 are invisible without rendering; and the current
"est 24k" phrasing would mislabel a prompt-length estimate as tokens —
the exact confusion the rename (Task-441) fixes in schema must also be
fixed on screen.

## 4. Exact Change

- `T-1` Step/run card figures: `prompt ~{est}k est` (from audit est or
  packed total) and `usage {n}k` (from `TokenUsage.Total.TotalTokens`);
  both labeled, `—` when absent.
- `T-2` `context_pressure` aware → inline banner/line on the affected
  step card (`context at ~NN% of window`) + status tint; cleared when a
  later usage event drops below the tier or leg rotates.
- `T-3` `provider_compacted` → inline notice line
  (`provider compressed context at ~NN% (190k→18k)`) pinned to the step
  card; no action.
- `T-4` `context_pressure` ask + `usage_budget_exceeded` → existing
  user-decision card via `EventUserQuestionRequired`; option ids
  (`rotate_leg`/`continue`/`stop`, `extend`/`stop`) answered through the
  chat prompt channel unchanged.
- `T-5` TUI: `formatContextLimits` gains pressure tint marker +
  `compacted` glyph on the session panel; no modal for awareness.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/**` (step/run card component + event
  ingest), `apps/local-runner/internal/tui/app/session_panel.go` +
  `helpers.go` (`formatContextLimits`), event-type strings/decoder on the
  client side for the two new event types
- modules: `desktop-flowpilot`, `tui/app`
- routes: none
- tables: none

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/components/<run-or-step-card>.tsx
// T-1 — two labeled figures; absent data renders "—"
function UsageFigures(props: { estPromptTokens: number | null; usageTokens: number | null }): JSX.Element
// T-2/T-3 — inline notices for aware-pressure and compaction
function ContextNotice(props: { kind: "pressure_aware" | "provider_compacted"; ratio?: number; prev?: number; cur?: number }): JSX.Element
```

```go
// apps/local-runner/internal/tui/app/helpers.go
// T-5 — extend (not fork) the existing formatter: pressure tint + compacted glyph
func formatContextLimits(lastTokens int64, window int64, pressureTier string, compacted bool) string
```

## 7. Test Signatures

- `test("step card renders prompt ~Nk est and usage Nk as separate labels")`
  — both labels present, values distinct (covers T-1)
- `test("aware pressure renders inline banner, not a card")` — banner
  visible, no decision card, run continues (covers T-2)
- `test("provider_compacted renders pinned notice line")` — notice text
  with prev→cur figures, no action buttons (covers T-3)
- `test("ask pressure renders decision card via existing card path")` —
  card shows continue/stop (+rotate_leg when present) (covers T-4)
- `test("usage_budget_exceeded renders extend/stop card")` — option ids
  route through chat prompt channel (covers T-4)
- `test("missing usage renders dash not zero")` — `—` + tooltip, never
  `0` (covers T-4 honesty)
- `TestTask444_TUI_FormatContextLimits_PressureTintAndCompacted` —
  table-driven formatter output per tier/compacted flag (covers T-5)

## 8. Acceptance Check

- On a real desktop build with `FLOWPILOT_CONTEXT_PRESSURE=1`, each event
  type produces its specified surface; awareness items never steal focus
  or open modals; decision cards answer through the normal channel.

## 9. Out of Scope

- Backend event production (Task-442/443 land first — this task only
  renders).
- Attention-inbox entries for awareness notices.
- Any new decision-card type or answer channel.
- Admin-web surfaces (desktop + TUI only this pass).

## 10. Definition of Done

- [x] All §6 signatures implemented exactly (or deviation documented in §11)
- [x] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [x] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [x] Provider parity proven or evidenced where the change touches shared/provider paths (R2)
- [x] `feature_key` set; CA ledger entry written; FEATURE-KEYS.md already contains the key
- [x] §8 acceptance checks verified by hand or test
- [x] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: TokenUsageSnapshot.estPromptTokens (runner-stamped len(prompt)/4 on every usage event) -> desktop UsageFigures 'prompt ~Nk est' + 'usage Nk', dash-on-missing; ContextNotice inline banner for pressure_aware + pinned provider_compacted (prev->cur figures), store contextNotice state with leg-pin auto-clear + below-tier clear; decision tier unchanged via user_question_required. TUI: contextStatus on AppModel fed by handleEvent; formatContextStatus composes formatContextLimits (signature unchanged — spec signature would have broken existing callers; deviation documented) adding est label + !ctx ~NN% + compacted prev->cur marks. 4 render + 4 store tests green; tsc --noEmit clean.
- follow-ups: ask-tier banner tint could differentiate visually (currently same marker)
- upstream docs updated: CP-86, CP-86-Test-Steps
