# Task-443: Context Pressure Ladder & Provider-Compaction Detection

- Document ID: `Task-443`
- Title: `Flag-gated context-pressure events (80% awareness / 90% ask_user) and provider self-compaction detection from usage drops`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-86`, `SD-10`, `SS-22`
- Child Documents: ``
- Related Documents: `Task-440` (window coverage — hard dependency), `Task-442` (shared `context_usage.go`), `CP-23 Task-335` (drift flag/ladder pattern)
- Replaces: ``
- Tags: `token-usage`, `context-pressure`, `compaction`, `escalation`

## AI Quick View

### Summary

- The runner records `Last.TotalTokens` + `ModelContextWindow` per turn but
  nobody consumes the ratio. When the provider hits its window it compacts
  internally — opaque to FlowPilot — and the user just sees a counter reset.
- Two detectable signals already exist in `rs.events`: `usage/window` ratio
  crossing thresholds, and `TotalTokens` dropping sharply **within one leg**
  (= provider compacted). Both become typed, durable ProviderEvents.
- Pressure is **awareness-first**: ≥80% emits a notice only; ≥90% emits an
  ask_user card (`rotate_leg` / `continue` / `stop`); compaction emits a
  notice line. Only caps ask — warnings never interrupt.

### Current Ask

- New `context_pressure.go` + two event types
  (`context_pressure` / `provider_compacted`), evaluated in `emitLocked`
  for each usage event, behind `FLOWPILOT_CONTEXT_PRESSURE` (default OFF,
  byte-identical).

### Key Decisions

- `T-1` Ratio = `Last.TotalTokens / ModelContextWindow` **only when both
  are known** — unknown window → evaluate nothing (never guess; matches
  `lastTurnTokensConsumed` degrade-soft convention).
- `T-2` Tiers: `<80%` none · `80–89%` `EventContextPressure{tier:"aware"}`
  · `≥90%` `EventContextPressure{tier:"ask"}` + user-decision card with
  `rotate_leg`/`continue`/`stop`. Deduped per leg — a tier fires once per
  leg, not per event.
- `T-3` Compaction = `prev.TotalTokens - cur.TotalTokens > threshold`
  (default >30% drop) **and** same `providerSessionID`/leg — drops across
  legs (model/provider switch) are normal resets, never compacted events.
- `T-4` `rotate_leg` uses the leg-reset path (provider-switch/new-leg
  mechanics with the **same** provider+account+model binding — a context
  reset, not routing). No candidate selection, no cooldown; it counts
  against a per-run leg-rotation cap. A headroom check on the pinned
  account still runs because re-seed costs tokens — on failure the
  request escalates into the CP-87 routing gate. If the new-leg path is
  unreachable for the run kind, the option is omitted and the card
  offers `continue`/`stop` only (typed degradation — see CP-86 Open
  Questions).

### Constraints

- Flag-gated `FLOWPILOT_CONTEXT_PRESSURE`, default OFF; flag-off path must
  be byte-identical (same rollout contract as Task-334/335).
- Evaluation inside `emitLocked` must stay allocation-cheap: no I/O, no
  goroutines spawned per event — pure reads of `rs.events` tail + `rs`
  fields.
- Events persist via the normal `emitLocked` path (durable ndjson) — never
  emit pressure events for a run the events themselves would recurse into
  (guard: don't evaluate pressure on our own emitted events — only on
  `EventTokenUsageUpdated`).
- Provider parity: thresholds + detection run on normalized snapshots →
  parity proven by fake-adapter tests per provider.
- Leg reset scope: only meaningful where one provider session persists
  across multiple turns — the main hub session or a node with an
  internal multi-turn loop. Per-step sub-agent execution already gets a
  fresh context window per node (node = new leg), so on those runs the
  pressure path is awareness + degraded-marking only; offering
  `rotate_leg` where the next step gets a fresh session anyway is a
  no-op and must be suppressed. The reset value is stopping *compounding
  compaction loss* in long-lived sessions — each provider compaction
  summarizes the previous summary, so drift grows per cycle; a reset
  restores a deterministic baseline seeded from durable state.
- Provider compaction cannot be prevented or triggered on demand — no
  provider exposes that control. The only mitigations are pre-emptive leg
  rotation at ≥90% (before the provider is forced to compact) and
  after-the-fact detection. A `provider_compacted` event marks the leg
  `context_degraded`; the next turn admission may treat it like the ≥90%
  tier (offer/auto `rotate_leg`) because provider-held context is no
  longer trustworthy. Detection itself is heuristic: providers that do
  not report per-turn context occupancy stay undetectable (degrade-soft).
- Rotation is never mid-turn: a `rotate_leg` answer commits durably but
  takes effect only at the next turn admission / child-spawn boundary —
  the in-flight turn always runs to its natural end first.
- Rotation never replays committed work: the pending intent binds the
  *next* admission to a new leg. If the node/run completes first, the
  intent expires — a finished step is never re-run for context reasons;
  its output quality is judged by the flow's own gates/review, not by
  the pressure system.
- `continue` is a legitimate terminal answer, not a degraded fallback:
  the provider either auto-compacts when it must (accepted — leg marked
  degraded on detection; provider, not FlowPilot, chooses what context
  survives) or hard-fails with a context-overflow error, which surfaces
  as a typed turn failure and re-enters the same routing gate.

### Open Questions

- Compaction drop threshold: 30% default — may need per-provider tuning
  since `Total` semantics differ (context-size vs cumulative); verify with
  real event shapes in tests before finalizing.

### Source Refs

- `CP-86 P-4`, `provider_event.go` (event enum, `TokenUsageSnapshot`),
  `interactive_service.go` (`emitLocked`), `gate_hook.go`
  (`lastTurnTokensConsumed`, `driftDetectorEnabled` — flag pattern),
  `driftdetect` (correction-ladder precedent).

## 1. Goal

Provider window pressure stops being invisible: every threshold crossing
and every silent provider compaction becomes a durable, typed event the UI
can surface — with the user deciding at the only real decision point (≥90%
or cap), never the engine.

## 2. Parent Links

- coding plan: `CP-86` (P-4)
- tech design: `SD-10`
- system spec: `SS-22`
- specific upstream ids: `Task-440`, `Task-442`, `CP-23 Task-335`

## 3. Trigger

Providers compact on their own terms (Devin `history_*.md`, Claude
auto-compact). FlowPilot's durable transcript survives, but the runner has
no record it happened — quality degradation and counter resets are
unexplainable to the user today.

## 4. Exact Change

- `T-1` New event types in `provider_event.go`:
  `EventContextPressure = "context_pressure"` and
  `EventProviderCompacted = "provider_compacted"` + payload fields on
  `ProviderEvent` (pressure tier, ratio, prev/cur tokens, leg/session id).
- `T-2` `context_pressure.go`: `evalContextPressureLocked(rs, snap)` —
  tier computation, per-leg dedupe (`rs.pressureTierFired`), compaction
  drop detection vs previous usage event in same leg.
- `T-3` Hook in `emitLocked`: after the usage-event append, when flag ON
  and `ev.Type == EventTokenUsageUpdated`, evaluate and emit follow-up
  events through the same `emitLocked` (guarded non-recursive — usage
  events only).
- `T-4` `tier:"ask"` emits `EventUserQuestionRequired` (existing card
  path) with `context_pressure_90` options: `rotate_leg` (when reachable),
  `continue`, `stop`.

## 5. Touched Areas

- files: `internal/runner/provider_event.go` (enum + fields),
  `internal/runner/context_pressure.go` (new),
  `internal/runner/interactive_service.go` (emitLocked hook + run fields),
  `internal/runner/decision_payload.go` (option ids)
- modules: `runner`
- routes: none (events flow on the existing stream)
- tables: none

## 6. Code Guide Signatures

```go
// internal/runner/provider_event.go
EventContextPressure   ProviderEventType = "context_pressure"    // T-1
EventProviderCompacted ProviderEventType = "provider_compacted"  // T-1

// ProviderEvent additions:
ContextPressure *ContextPressurePayload `json:"contextPressure,omitempty"`

type ContextPressurePayload struct {
    Tier           string  `json:"tier"`            // "aware" | "ask"
    Ratio          float64 `json:"ratio"`           // usage/window
    UsedTokens     int64   `json:"usedTokens"`
    WindowTokens   int64   `json:"windowTokens"`
    PrevTokens     int64   `json:"prevTokens,omitempty"` // compaction only
    LegID          string  `json:"legId,omitempty"`
}
```

```go
// internal/runner/context_pressure.go (new)
const contextPressureEnvFlag = "FLOWPILOT_CONTEXT_PRESSURE"
func contextPressureEnabled() bool
func (s *InteractiveService) evalContextPressureLocked(rs *interactiveRun, snap *TokenUsageSnapshot)
func pressureTier(ratio float64) string // "" | "aware" | "ask"
func isProviderCompaction(prev, cur int64) bool // >30% drop, same leg
```

```go
// internal/runner/interactive_service.go — interactiveRun additions:
pressureTierFired map[string]bool // per-leg tier dedupe (T-2)
lastUsageTokens   int64           // previous TotalTokens for drop detection (T-3)
```

## 7. Test Signatures

- `TestTask443_FlagOff_NoPressureEvents` — usage ≥95% with flag off → zero
  new event types (covers rollout contract)
- `TestTask443_Tier80_EmitsAwarenessOnly` — 80–89% → `context_pressure`
  tier=aware, no ask card (covers T-2)
- `TestTask443_Tier90_EmitsAskUserCard` — ≥90% → tier=ask +
  `user_question_required` with continue/stop (+rotate_leg when reachable)
  (covers T-2/T-4)
- `TestTask443_TierFiresOncePerLeg` — repeated usage events at 85% → one
  event, not one per update (covers T-2 dedupe)
- `TestTask443_UnknownWindow_Silent` — nil window → no events (covers T-1)
- `TestTask443_TokenDropSameLeg_EmitsProviderCompacted` — 190k→18k same
  session → `provider_compacted` with prev/cur (covers T-3)
- `TestTask443_PostCompaction_LegMarkedDegraded` — after
  `provider_compacted`, the leg carries `context_degraded` so next
  admission can offer/auto rotate (covers constraint degradation)
- `TestTask443_RotateLegKeepsSameBinding` — answer `rotate_leg` → new
  leg keeps identical provider/model/account binding; no candidate
  routing involved (covers T-4)
- `TestTask443_TokenDropAcrossLeg_NotCompacted` — drop across
  providerSessionID change → nothing (covers T-3 false-positive guard)
- `TestTask443_EventsPersistedToNdjson` — emitted pressure/compaction
  events present in persisted event log (covers durability)
- `TestTask443_ClaudeCodexGrok_Parity` — fake-adapter table across all
  three providers (covers AGENTS §5)

## 8. Acceptance Check

- With flag ON, a run driven past 80%/90% emits exactly the specified
  events in order, durable and deduped per leg; flag OFF produces zero
  behavioral delta.
- A simulated provider compaction produces a `provider_compacted` event —
  never an ask card.

## 9. Out of Scope

- Automatic leg rotation mechanics (the `rotate_leg` answer reuses the
  existing provider-switch path; new auto-rotation machinery is a later CP).
- Mid-turn intervention of any kind.
- UI rendering of these events (Task-444).
- Gemini (no usage events — documented exclusion).

## 10. Definition of Done

- [x] All §6 signatures implemented exactly (or deviation documented in §11)
- [x] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [x] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [x] Provider parity proven or evidenced where the change touches shared/provider paths (R2)
- [x] `feature_key` set; CA ledger entry written; FEATURE-KEYS.md already contains the key
- [x] §8 acceptance checks verified by hand or test
- [x] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: FLOWPILOT_CONTEXT_PRESSURE-gated ladder: aware ~80% -> context_pressure event; ask ~90% -> durable question (rotate_leg/continue/stop) persisted-before-emit; tiers fire once per leg; unknown window stays silent; same-leg >30% TotalTokens drop -> provider_compacted + leg marked context-degraded; cross-leg drops ignored; rotate_leg = same-binding leg reset via switchChatLeg(allowSameProvider) consumed at next turn admission only (never mid-turn), child runs suppressed; contextResetHeadroomOK seam for CP-87 quota preflight. 12/12 task tests green incl. persisted-event assertion.
- follow-ups: CP-87 supplies real headroom feed; provider_status compaction-free providers documented
- upstream docs updated: CP-86, CP-86-Test-Steps
