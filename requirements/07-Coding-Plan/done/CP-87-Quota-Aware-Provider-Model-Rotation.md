# CP-87: Quota-Aware Provider / Model Rotation

- Document ID: `CP-87`
- Title: `Typed provider-limit handling, quota preflight, workload-class model routing, and user-controlled rotation`
- Phase: `coding_plan`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-86`, `SD-07` (Skill & Agent Runtime), `SD-10` (Context Resolver & RAG), `SD-17` (Context & Regression Engine), `SS-06` (Workflow Skill Agent), `SS-22` (Runtime Intelligence & Drift Correction)
- Child Documents: `Task-445`, `Task-446`, `Task-447`, `Task-448`, `Task-449`, `Task-450`, `CP-87-Test-Steps`
- Related Documents: `Task-320` (per-node model resolution), `CP-70` (Devin provider), `CP-57` (OpenCode provider), `CP-46` (Grok provider), `CP-84` (attention/decision UX), `decision-card-ui`
- Replaces: ``
- Tags: `quota`, `provider-routing`, `model-routing`, `account-pool`, `vibe`, `flow`

## AI Quick View

### Summary

- Quota exhaustion is currently detected twice by string matching: Go marks
  the turn non-recoverable; Desktop repeats the same token list to decide
  whether to show an account-switch prompt. Provider wording drift has already
  caused real misses (BUG-361, BUG-374).
- CP-87 normalizes provider failures once at the adapter/runner boundary into
  typed `provider_limit_reached` events. Claude/Codex are fixture-contract
  verified because this machine has no live accounts; Grok/Devin success +
  quota metadata can be live-verified; OpenCode can use free success-path plus
  fake ACP limit fixtures. Evidence levels remain explicit — fixture coverage
  is never labeled live parity.
- Before any provider-backed hub/node starts, a quota preflight resolves the
  effective provider/model/account. It tries a healthy same-provider account
  first; if none qualify, it dynamically evaluates every other registered
  provider (`provider != current`) — no hardcoded provider list.
- Cross-provider equivalence is **workload-class based**, not pairwise model
  mapping: `scan` (cheap/fast), `high_reasoning` (plan/review/TDD; strongest),
  `coding` (normal implementation model because plan/scaffold already guide
  it). Flow/vibe provider nodes declare a class; settings bind each
  provider+class to a preferred model and define provider priority.
- Rotation is user-controlled. Machine-global `quotaRotationMode` defaults
  `manual`: Flow and fully-automatic Vibe both park at a quota gate and show a
  candidate table. `auto` lets the runner choose only a high-confidence
  candidate; unknown/stale/ambiguous candidates still gate the user.

### Current Ask

- Build a provider-neutral quota routing plane that catches known provider
  limit shapes once, preflights account headroom before hub/sub-agent work,
  rotates safely according to user settings and workload class, preserves
  provider/session/account leg invariants, and records requested→resolved
  routing + actual usage for audit.

### Key Decisions

- `P-1` Typed limit taxonomy:
  `quota_exhausted | rate_limited | credits_exhausted | billing_required`.
  Adapter structured payload/stop reason wins; centralized string fallback is
  last-resort with `confidence: heuristic`. Desktop never parses limit strings.
- `P-2` Quota telemetry is not renamed "tokens": account APIs expose
  percentages/credits/windows with provider-specific semantics. Normalize to
  `healthy | low | exhausted | unknown | stale` + source/freshness/confidence.
  `maxUsageTokens` from CP-86 informs expected node cost but is never directly
  compared to an account percentage.
- `P-3` Effective execution demand:
  - main hub uses its pinned `providerKey/modelName/providerAccountID`;
  - child node uses existing model precedence (DB step override > YAML model >
    agent/inherit), derives provider from model, then selects account;
  - changing account or provider always creates a new leg; never mutate a live
    provider session in place.
- `P-3b` The routing gate consumes two trigger kinds: `provider_limit`
  (typed event — the failed account is excluded) and
  `usage_budget_exceeded` (CP-86 post-turn cap — adds a manual-only
  `extend` action alongside rotate candidates). `context_pressure` is
  **not** a routing trigger — it is a leg-lifecycle decision owned by
  CP-86: the resolution is deterministically a new leg on the same
  provider+account+model binding (a context reset; no candidate
  selection, no cooldown). Routing is entered only when that
  same-binding reset fails the headroom check — the re-seed handoff
  costs tokens — at which point the request escalates into the normal
  `provider_limit`-style candidate flow.
- `P-3c` Two distinct problems share only the new-leg machinery —
  terminology must stay separate: **account/provider rotation** fixes
  quota exhaustion (a different account supplies budget — the router
  owns candidate selection); **leg reset** fixes a full or compacted
  context window (a fresh session on the same binding — leg lifecycle
  owns it, there are no candidates). The reset path only performs a
  headroom check on the pinned account for the re-seed cost; a failed
  check escalates into the routing gate instead of resetting into
  immediate failure.
- `P-4` Same-provider account selection precedes cross-provider fallback, but
  automatic same-provider rotation is rate-safe: minimum **20s cooldown** between
  switches for one provider + max 2 automatic same-provider switches per run;
  then gate the user. During the cooldown, Desktop/TUI show a visible countdown
  progress bar and the safety reason: rapid switching across several accounts
  of one provider on the same IP can trigger provider abuse/risk controls. No
  tight account cycling or hidden background wait. The cooldown governs
  **account switches** only — a new leg on the same account
  (context-pressure rotation) is not rate-limited and is bounded by a
  separate max-2 automatic leg rotations per run.
- `P-5` `quotaRotationMode: manual | auto`, stored machine-global because
  provider accounts/homes are machine-local; default `manual`. Snapshot the
  mode into each run at start so a settings edit cannot change an active run
  mid-flight. Vibe does not bypass this: manual mode always gates; auto mode
  auto-selects only exact/high-confidence candidates.
- `P-6` Cross-provider routing uses explicit data, not pairwise code:
  `workloadClass` on provider-backed nodes + settings table
  `(provider, workloadClass) -> preferred model`; ordered provider priority.
  A candidate must also satisfy required capabilities and context window.
  New providers participate by catalog/settings data without routing-code edits.
- `P-7` Rotation is an execution override only — durable per run/step/leg and
  never silently updates Supabase `step_definitions.model`. The candidate UI
  may offer an explicit `Save as step default` action; only that user action
  mutates the base step.
- `P-8` Auto mode is deterministic: same-provider healthy account > configured
  provider priority > workload-class preferred model > headroom/freshness >
  account slot tie-break. Unknown/stale quota or missing class/model binding
  always falls back to the user gate. Auto resolves the routing triggers
  (`provider_limit`, `usage_budget_exceeded`) only via high-confidence
  candidates; the budget `extend` action is never auto-selected — it
  changes the user's declared cap. Context pressure has its own auto
  path — the deterministic same-binding leg reset — still governed by
  manual/auto mode and bounded by the leg-rotation cap.

### Constraints

- `SetActiveAccount` is global and interrupts all in-flight turns. CP-87 must
  not use it for automatic routing. Account selection must be pinned per
  run/leg and handed to provider process/session creation.
- Automatic rotation must not be used to evade provider policy. Respect reset
  times, cooldowns, billing-required blocks, configured provider terms, and a
  bounded rotation cap; never cycle indefinitely through accounts.
- In-flight turns finish or fail naturally; preflight occurs before a new turn
  or child spawn. No mid-turn destructive switch.
- Additive tests; provider-specific fixtures are sanitized and contain no
  secrets. No live Claude/Codex account exists on this machine.
- Gemini is out of scope (not maintained by the user). Claude, Codex, Grok,
  OpenCode, and Devin are the required provider matrix.
- Flow and Vibe must share one resolver/gate path; no separate Vibe-only quota
  behavior.

### Open Questions

- Provider-specific quota freshness TTL defaults: proposed 2 minutes for
  preflight and immediate refresh after a limit event; confirm during Task-446.
- `Save as step default` may be deferred from Task-450 if Supabase write
  authority makes the UI slice too broad; run/step override remains mandatory.

### Source Refs

- `CP-86` + `Task-442`/`Task-443` (`usage_budget_exceeded` and
  `context_pressure` signals feed this gate), `Task-320`
  (`resolveFlowNodeModel`, `ModelProviderKey`),
  `interactive_service.go:isProviderUsageLimitError`,
  `claude_event_mapper.go:claudeUsageLimitMessage`,
  `opencode_event_mapper.go:opencodeIsQuotaStopReason`,
  `devin_event_mapper.go:devinIsQuotaStopReason`,
  `store.ts:isUsageLimitMessage/findBestCandidate/confirmAccountSwitch`,
  `interactive_service.go:SetActiveAccount` (global interruption hazard),
  `flow_executor.go:delegateSpawnModel`.

## 1. Goal

Turn quota exhaustion from duplicated string heuristics and reactive account
switching into a typed, durable, provider-neutral routing contract that can
preflight every Flow/Vibe provider node, ask the user by default, and safely
auto-rotate when explicitly enabled.

## 2. Input Documents

- `requirements/07-Coding-Plan/todo/CP-86-Real-Usage-Accounting-And-Context-Pressure.md`
- `requirements/08-Task/inprogress/Task-320-Per-Node-Model-Tiering-For-Harness-Delegate-Nodes.md`
- `requirements/06-System-Tech-Design/SD-07-Skill-Agent-Runtime.md`
- `requirements/06-System-Tech-Design/SD-10-Context-Resolver-RAG.md`
- `requirements/06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md`
- `requirements/05-System-Specs/SS-22-Runtime-Intelligence-And-Drift-Correction.md`

## 3. Implementation Strategy

- Normalize limit errors at provider ingress and emit one typed event; retire
  Desktop's parallel string classifier only after typed-event parity tests land.
- Add data contracts before policy: workload classes/model bindings, normalized
  headroom, routing mode, provider priority, account cooldown state.
- Introduce a single `QuotaPreflightResolver` called by root/hub turn admission
  and every child spawn seam. It returns `proceed | gate | rotate | blocked`
  with a complete, auditable resolution — never performs UI or persistence
  directly.
- Manual mode parks through the existing decision-card/attention infrastructure;
  auto mode consumes the same candidate list and resolution API.
- Account/provider changes create a new pinned leg with a compact durable
  handoff; base workflow definitions remain unchanged.

## 4. Work Breakdown

- `P-1` Task-445 — provider-limit taxonomy + typed event; adapter fixtures for
  Claude/Codex/Grok/OpenCode/Devin; remove Desktop string matching.
- `P-2` Task-446 — workload-class schema, model bindings, provider priority,
  normalized quota headroom and machine-global rotation setting.
- `P-3` Task-447 — effective demand resolver + same-provider account pool,
  cooldown/rotation caps, per-leg account pinning (no global SetActiveAccount).
- `P-4` Task-448 — cross-provider candidate engine: all registered providers,
  workload class/capabilities/context-window filters, deterministic ranking.
- `P-5` Task-449 — Flow + Vibe admission integration: manual gate, auto
  rotation, new-leg handoff, retry/stop semantics and crash-safe persistence.
- `P-6` Task-450 — Desktop/TUI settings + candidate table + routing audit UI;
  explicit optional Save-as-step-default.

## 5. Touched Areas

- files: provider event/mappers/adapters, `interactive_service.go`,
  `flow_executor.go`, `agentpack/pack.go`, flow-pack YAMLs, provider account
  metadata APIs, new `quota`/`routing` runner package or files, desktop store +
  settings + decision controls, TUI client/app.
- modules: `runner`, `agentpack`, Desktop state/components, TUI.
- database: optional explicit Save-as-default uses existing
  `step_definitions.model`; no automatic mutation. Runtime decisions remain in
  durable run/leg event state.
- external systems: local provider CLIs/APIs for quota metadata only.

## 6. Data or Migration Steps

- flow pack schema: provider-backed nodes gain required `workloadClass`:
  `scan | high_reasoning | coding`; inline/control nodes omit it.
- settings schema (machine-global runner config):
  `quotaRotationMode`, `providerPriority`, per-provider/per-class model binding,
  quota thresholds/freshness TTL, same-provider cooldown.
- run snapshot: resolved routing mode/policy + per-leg account/model/provider
  decision persisted for restart/replay.
- no destructive DB migration.

## 7. Validation Plan

- tests to add: typed error fixtures for five providers; unknown-shape fallback;
  headroom normalization; hub vs child effective demand; same-provider account
  selection/cooldown/cap; dynamic cross-provider candidate discovery; workload
  class model selection; manual/auto parity in Flow and Vibe; leg pin/recovery;
  UI table/settings/audit rendering.
- live checks: Grok + Devin success/quota metadata; OpenCode free success path.
  Claude/Codex marked fixture-contract-only until live accounts exist.
- failure cases: telemetry unknown/stale; no candidate; all accounts exhausted;
  rate-limit with Retry-After; billing-required; concurrent nodes racing for one
  account; crash after selection before provider call; restart while gated.

## 8. Rollout and Fallback

- rollout order: typed events → settings/schema → same-provider resolver →
  cross-provider candidates → Flow/Vibe integration → UI.
- default `manual` means no automatic provider/account change on rollout.
- fallback: disable quota preflight feature flag; typed event/audit remains
  observational. Legacy turn failure still surfaces, but Desktop must not
  restore duplicate string matching after typed cutover.
- monitoring: routing decision audit, limit kind/source/confidence, candidate
  rejection reasons, cooldown blocks, requested→resolved model/provider/account,
  estimated prompt + actual usage from CP-86.

## 9. Risks

- `R-1` Provider quota shapes change — typed boundary centralizes the update;
  unclassified payloads audit safely instead of being retried indefinitely.
- `R-2` Account percentage is not token capacity — normalized headroom and
  confidence prevent false precision; unknown/stale gates instead of auto.
- `R-3` Global account switching interrupts unrelated runs — prohibited;
  Task-447 must establish per-leg account pinning before auto mode can ship.
- `R-4` Model class mapping can become stale — explicit settings/catalog data,
  no hardcoded pairwise matrix; auto requires exact binding.
- `R-5` Parallel nodes overbook one account — account selection uses a durable
  claim/lease or serialized reservation and rechecks immediately before call.

## 10. Definition of Done

- [x] One typed provider-limit schema covers known Claude/Codex/Grok/OpenCode/
      Devin shapes; Desktop contains no quota-string classifier. (Task-445,
      CA-977 — `provider_limit_reached` + `ProviderLimitKind`; Desktop consumes
      the typed event.)
- [x] All provider-backed Flow/Vibe nodes declare one workload class; model
      bindings/provider priority are data-driven and validated fail-closed.
      (Task-446, CA-978 — 52 provider-backed nodes annotated; bindings +
      priority live in machine-global `QuotaRoutingSettings`.)
- [x] Manual mode (default) gates both Flow and Vibe with a candidate table;
      auto mode rotates only high-confidence candidates and otherwise gates.
      (Task-449, CA-981 — shared `quota_route_required` gate; auto restricted
      to exact/fresh/healthy candidates.)
- [x] Same-provider selection respects the 20s cooldown + max-two automatic
      switches/run; Desktop/TUI show a server-timestamp-driven countdown bar
      and safety reason; no global `SetActiveAccount` in routing. (Task-447,
      CA-979 — durable claim ledger + `same_provider_ip_safety`; Task-450,
      CA-982 — determinate server-deadline bars.)
- [x] Cross-provider resolver enumerates registry providers dynamically and
      enforces workload class, capability, context-window, account headroom.
      (Task-448, CA-980 — pure candidate engine, sixth-provider acceptance.)
- [x] Rotation creates a durable new leg; no live session mutation and no
      silent Supabase step-model update. (Task-449 — `switchChatLeg` /
      `claimAccountForLeg` / respawn paths only.)
- [x] Requested/resolved routing + quota evidence + CP-86 usage is audited.
      (Task-450 — `GET /client/workflow-runs/{id}/quota-audit` +
      `quota_route_committed` policy/headroom fields.)
- [x] Additive tests green; live/fixture evidence levels explicitly reported;
      GitNexus `detect_changes` clean before every commit (CLI lacks the tool;
      equivalent staged-diff scope reviews documented per CA).

## 11. Completion Notes

- Landed across Task-445..450 on branch `cp_86_87` (commits `b63600ee`,
  `6c03a09f`, `303eb490`, `c006c3db`, `1fee296a`, `d629a24e`; audits
  CA-977..CA-982).
- Evidence levels: Claude/Codex limit shapes are fixture-contract only (no
  live accounts on this machine); Grok/Devin success-path + quota metadata
  fixtures; OpenCode free-path + fake-ACP limit fixtures.
- Deferred: `Save as step default` (open question — hidden, not shipped);
  live-account smoke of the quota gate UI.
