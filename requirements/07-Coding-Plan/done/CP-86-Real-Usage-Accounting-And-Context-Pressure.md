# CP-86: Real Usage Accounting & Context Pressure Ladder

- Document ID: `CP-86`
- Title: `Separate est-prompt budget from real token usage; provider context-window coverage; pressure ladder + usage cap escalation`
- Phase: `coding_plan`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `SD-10` (Context Resolver & RAG), `SD-17` (Context & Regression Engine), `SS-09` (Artifact Memory & Context Retrieval), `SS-22` (Runtime Intelligence & Drift Correction)
- Child Documents: `Task-440`, `Task-441`, `Task-442`, `Task-443`, `Task-444`, `CP-86-Test-Steps`
- Related Documents: `CP-23` (Budget Packer Task-334, Drift Detector Task-335), `CP-62` (context profiles Task-341), `CP-67` (Contract-First TDD flows), `CP-87` (routing gate consumes the budget/pressure signals)
- Tags: `context`, `token-usage`, `budget`, `provider-parity`, `escalation`

## AI Quick View

### Summary

- Today `contextProfiles.*.maxTokens` is a **prompt-length estimate cap**
  (bytes/4 heuristic, pre-send) but its name says "token" — operators read it
  as real usage. Two different budgets are being conflated: est input length
  vs provider-reported consumption.
- Every adapter already emits `EventTokenUsageUpdated` →
  `TokenUsageSnapshot{Last, Total, ModelContextWindow}` — but nothing
  *consumes* window pressure: no threshold, no typed event, no UX. When a
  provider silently compacts (usage drops 190k→18k) the runner records
  nothing and the user sees a counter reset that looks like a bug.
- Claude emits usage but **no `ModelContextWindow`** (Claude CLI never sends
  it — window is a catalog constant). The runner already serves this catalog
  to clients (`ProviderModel.ContextWindowTokens`) but never fills it into
  usage snapshots. Gemini adapter emits no usage at all — **explicitly out of
  scope** for this CP.
- `maxUsageTokens` (new) is the real post-run consumption budget per node —
  only comparable after the provider reports usage each turn. Exceed →
  the shared routing gate (CP-87): `extend` / `rotate` / `stop` in manual
  mode; auto mode may rotate a high-confidence candidate but never
  auto-extends. Reaching a cap is **not a bug**: it is a decision point
  owned by the user.

### Current Ask

- Land the accounting + pressure layer so a flow can never silently burn past
  the provider's window or an unbudgeted amount of real tokens: window
  coverage for Claude, honest naming (`maxEstPromptTokens`), a real
  `maxUsageTokens` cap with ask_user escalation, and a flag-gated pressure
  ladder (≥80% awareness, ≥90% ask_user, provider-compaction notice).

### Key Decisions

- `P-1` One enrichment seam: `emitLocked` fills
  `TokenUsage.ModelContextWindow` from the runner's provider model catalog
  (`rs.providerKey` + `rs.modelName`) whenever the adapter did not
  self-report. All downstream consumers (TUI, drift, pressure) read one
  uniform value. Self-reported windows are never overridden; unknown → nil →
  deterministic degradation (no fake numbers).
- `P-2` Hard rename `contextProfiles.*.maxTokens` → `maxEstPromptTokens`
  (YAML key + `agentpack.ContextProfile.MaxTokens` → `MaxEstPromptTokens`).
  No alias: all builtin flows are repo-owned and updated in the same pass.
  Go symbol rename goes through `gitnexus_rename`, never find-and-replace.
- `P-3` `maxUsageTokens` on the same profile = real consumption cap per node
  invocation, measured on `Total.TotalTokens` accumulated from usage events.
  Exceed → the in-flight turn finishes (never killed mid-turn), then a
  structured `ask_user` escalation offers `extend` (user-only — it changes
  the declared budget) / `rotate` (CP-87 routing gate — auto mode may pick
  a high-confidence candidate, bounded by the per-run rotation cap; the
  rotation applies a one-time extension so the check does not re-fire) /
  `stop`. Usage accounting carries across legs of the same node run —
  rotation never resets the counter.
- `P-4` Context pressure ladder behind `FLOWPILOT_CONTEXT_PRESSURE` (default
  OFF, byte-identical rollout like the budget packer): `≥80%` →
  `EventContextPressure` awareness only; `≥90%` → `ask_user` card
  (`rotate_leg` via the CP-87 gate — a new leg on the same
  provider+account is the default candidate — / `continue` / `stop`);
  `TotalTokens` drop within one leg → `EventProviderCompacted` notice and
  the leg is marked `context_degraded` (next admission may offer/auto
  rotate). Window unknown → silent, never guessed.
- `P-5` UI honesty: step card shows `prompt ~Nk est` vs `usage Nk` as two
  explicitly-labeled numbers — never one "est tokens" figure; compacted and
  pressure states are inline notices, only hard caps become ask_user cards.

### Constraints

- Additive tests only; fake provider adapter + fake catalog — no live Claude
  account required (none available on the dev machine).
- Provider parity (AGENTS §5): every change touching provider paths is
  proven across Claude / Codex / Grok or evidenced provider-agnostic.
  **Gemini is excluded** — the adapter emits no usage events and is not
  maintained in this CP.
- Pressure detector ships flag-gated OFF; default behavior byte-identical.
- Reaching `maxUsageTokens` or the pressure ladder is a **decision point,
  not a bug** — the only blocking surface is `ask_user`; awareness tiers
  never interrupt.
- Manual mode never rotates without user choice; auto mode (CP-87 opt-in)
  rotates only high-confidence candidates within the per-run cap. No
  mid-turn kills.
- `emitLocked` runs under `s.mu` — enrichment must be pure in-memory lookup,
  no I/O, no blocking calls inside the lock.

### Open Questions

- Leg rotation execution (new provider session seeded from artifacts) is
  offered as an `ask_user` option at ≥90%, but the rotation mechanics reuse
  the existing provider-switch leg path — confirm that path is reachable for
  child flow runs or descope to `stop`/`continue` only for MVP.
- `Total.TotalTokens` semantics differ slightly per adapter (cumulative vs
  context-size) — the pressure ratio MUST use the field each adapter treats
  as "current window occupancy"; verify per-adapter mapping in Task-443
  before wiring thresholds.

### Source Refs

- `SD-10` §5/§7 (context slots, prompt packing tiers), `SD-17` (context +
  regression engine), `SS-09` §4-§5 (prompt memory, retrieval rules),
  `SS-22` (runtime intelligence), `CP-23` Task-334 (budget packer), `CP-62`
  Task-341 (context profiles), `agentpack/pack.go` (`ContextProfile`),
  `context_profile.go` (`flowNodeProfileBudgetFor`),
  `interactive_service.go` (`emitLocked`, `lastTurnTokensConsumed`,
  `applyBudgetPackerIfEnabled`), `provider_event.go` (`TokenUsageSnapshot`).

## 1. Goal

Make token accounting honest and actionable: separate the prompt-length
estimate cap from real provider-reported usage, give every supported provider
a resolvable context window, and turn window pressure + usage overrun into
typed events the user can see and decide on — instead of silent provider-side
compaction.

## 2. Input Documents

- `requirements/06-System-Tech-Design/SD-10-Context-Resolver-RAG.md`
- `requirements/06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md`
- `requirements/05-System-Specs/SS-09-Artifact-Memory-Context-Retrieval.md`
- `requirements/05-System-Specs/SS-22-Runtime-Intelligence-And-Drift-Correction.md`
- `requirements/07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md` (context profiles)

## 3. Implementation Strategy

- Single enrichment seam over per-consumer fallbacks: fix window coverage
  once in `emitLocked` so TUI, drift, and the new pressure detector all read
  the same `ModelContextWindow`.
- Naming fix before new semantics: rename the est cap first so `maxUsageTokens`
  lands in a schema where "token" unambiguously means provider-reported.
- Reuse existing escalation surfaces (`when: escalate` edges,
  `user_decision_card` / `EventUserQuestionRequired`) instead of inventing a
  new interrupt channel.
- Everything new is flag-gated or additive; the pre-send estimator stays
  heuristic (bytes/4 + per-kind multipliers) — no tokenizer dependency.

## 4. Work Breakdown

- `P-1` Task-440 — `ModelContextWindow` catalog fallback at `emitLocked`
  (Claude coverage; never override self-reported; nil when unknown).
- `P-2` Task-441 — `maxTokens` → `maxEstPromptTokens` hard rename
  (agentpack struct + YAML flows + runner read path + tests + docs).
- `P-3` Task-442 — `maxUsageTokens` profile field + per-node real-usage
  accounting + exceed → `ask_user` (extend/stop).
- `P-4` Task-443 — pressure ladder + compaction detection:
  `EventContextPressure` / `EventProviderCompacted`, flag-gated.
- `P-5` Task-444 — UI surface: desktop step card (est vs usage labels,
  compacted notice, pressure banner) + TUI status; ask_user cards reuse the
  decision-card path.

## 5. Touched Areas

- files: `agentpack/pack.go`, `runner/context_profile.go`,
  `runner/interactive_service.go` (`emitLocked`), `runner/provider_event.go`,
  new `runner/context_pressure.go`, `runner/context_usage.go`,
  `agentpack/flow-pack/flows/*.yaml`, desktop `Navigator`/run-view +
  step-card components, TUI status/session panel.
- modules: `agentpack`, `runner`, `promptpacker` (audit only), `tui/app`,
  `desktop-flowpilot`.
- database: none (events persist via existing run-events ndjson).
- external systems: none — all provider data already flows through
  `EventTokenUsageUpdated`.

## 6. Data or Migration Steps

- schema: flow YAML key `maxTokens` → `maxEstPromptTokens` (all builtin
  flows updated atomically; no alias, no backfill — repo-owned configs only).
- data backfill: none.
- config updates: new opt-in env flag `FLOWPILOT_CONTEXT_PRESSURE`
  (default OFF).

## 7. Validation Plan

- tests to add: window enrichment (self-report/override/unknown/parity),
  rename round-trip parse, usage accounting + exceed escalation, pressure
  tier transitions + compaction detection + flag-off byte-identity, UI
  render states.
- manual checks: desktop step card labels est vs usage distinctly; pressure
  banner and compacted notice render from real events; `ask_user` card
  options route correctly.
- failure cases: unknown window → pressure detector silent; provider never
  emits usage → usage cap never fires (degrade-soft, audit notes it); pack
  or pressure code panics → caught, prompt/turn unaffected.

## 8. Rollout and Fallback

- rollout order: P-1 (window) → P-2 (rename) → P-3 (usage cap) →
  P-4 (pressure, flag OFF) → P-5 (UI). Each task independently revertable.
- fallback path: unset `FLOWPILOT_CONTEXT_PRESSURE` restores pre-P-4
  behavior; P-1..P-3 are additive/rename-only and revert cleanly.
- monitoring: `prompt-context-audit-*.jsonl` gains actual-usage fields;
  `[context-pressure]` log lines mirror the `[drift]` / `[prompt-pack]`
  conventions.

## 9. Risks

- `R-1` Hard rename breaks any out-of-repo flow YAML still using `maxTokens`
  — accepted: builtin flows are the supported surface; parse error is
  fail-closed and loud (unknown key surfaces immediately at flow load).
- `R-2` `Total.TotalTokens` semantic drift across adapters could fire
  pressure/compaction falsely — mitigated by per-adapter mapping verified in
  Task-443 tests and the flag-gate.
- `R-3` ask_user at ≥90% during unattended flow runs parks a node waiting on
  the user — acceptable per "decision point, not bug"; awareness tier at 80%
  gives earlier visibility.

## 10. Definition of Done

- [x] Every `token_usage_updated` event carries a `ModelContextWindow` when
      the catalog knows the model — Claude covered, self-reported values
      never overridden, unknown stays nil. (Task-440, 5/5 green)
- [x] `maxEstPromptTokens` is the only accepted profile key; `maxTokens`
      fails flow load loudly; all builtin flows + tests + docs updated.
      (Task-441, 4/4 green)
- [x] `maxUsageTokens` enforced post-turn → structured `ask_user`
      (extend/stop); in-flight turns never killed. (Task-442, 12/12 green)
- [x] Pressure ladder + compaction events emitted under flag, OFF by
      default; flag-off path byte-identical. (Task-443, 13/13 green —
      includes degraded-leg admission offer added in review)
- [x] Desktop + TUI render est/usage/compacted/pressure states distinctly;
      only caps produce ask_user cards. (Task-444, 8/8 desktop + 8/8 Go
      green, tsc clean)
- [x] Additive tests green across Claude/Codex/Grok fake adapters; no
      pre-existing test weakened (mechanical renames documented).
      Full runner suite: 113 fails all environmental — identical on clean
      HEAD (Windows TempDir cleanup locks, provider inventory 4→6,
      Supabase env vars, gitnexus auto-index). Desktop suite 571/581
      (10 pre-existing env fails, CA-969 baseline).
- [x] `feature_key` = `runtime-intelligence` + `token-usage`; CA ledger
      entries per task (CA-970..CA-974); GitNexus `detect_changes` clean
      before commit.
