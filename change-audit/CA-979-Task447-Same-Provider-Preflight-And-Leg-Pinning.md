# CA-979 — Task-447: same-provider account preflight + per-leg pinning

## Summary

The router can now see what an execution boundary needs (ExecutionDemand) and
rank the provider's connected accounts by normalized headroom
(AccountCandidate), then commit the chosen account as a durable claim that
pins the run's leg — without ever mutating the machine-global active account.
Same-provider switches mint a server-authoritative 20s cooldown
(`same_provider_ip_safety`) and automatic claims are capped at two per run.

## What changed

- `runner/quota_preflight.go` (new) — `ExecutionDemand` (hub leg vs flow-node
  child, resolved through `resolveFlowNodeModel`'s Task-320 precedence),
  `AccountCandidate` (headroom + autoEligible + machine-readable rejection
  reasons + cooldown projection), `ResolveExecutionDemand`,
  `SameProviderCandidates` (connected-only, auto = healthy+exact+fresh+not
  claimed+not tried+no cooldown+under cap; sorted auto-eligible → headroom →
  freshness → slot), `quotaNow`, `quotaSettingsForRun` (frozen per-run
  snapshot, defaults for pre-446 runs), `SetQuotaTelemetry`.
- `runner/quota_claim.go` (new) — durable machine-local ledger
  `quota-routing-state.json` (`FLOWPILOT_QUOTA_ROUTING_STATE_FILE` override):
  claims (active/superseded), same-provider switch records (started/until/
  reason), blocked accounts. `claimAccountForLeg` serializes under `quotaMu`,
  refuses claims during an active cooldown window, conflicts when another run
  holds the account, applies the max-2 automatic cap, re-validates
  connectivity at claim time, and pins the resident run + persists the
  session snapshot. `noteAccountBlockedLocked` records live
  billing_required / credits_exhausted events (Task-445) into the ledger.
- `provider_event.go` — `StartRunInput.{ProviderAccountID,AccountPinned,
  QuotaClaimID}`: the claim path (or a client minting a rotation leg) stamps
  the pin at run creation.
- `agent_orchestrator.go` — `SpawnAgentInput.ProviderAccountID` (internal).
- `interactive_handlers.go` — `createRun` honors an explicit
  ProviderAccountID over ambient active-account resolution.
- `interactive_service.go` — `interactiveRun.{accountPinned,quotaClaimID}`;
  service fields `quotaMu/quotaRuntimePath/quotaNowFn/quotaTelemetryFn`;
  `spawnChildRun` inherits a same-provider parent's pin (explicit claim wins);
  admission guard validates a pinned leg's account directly (fail-closed
  `account_unavailable`) instead of comparing to the global active account;
  adapter construction is account-scoped via `AdapterForAccount`;
  provider_limit emit records billing/credits blocks into the ledger.
- `interactive_resume.go` / `workflow_store.go` —
  `ProviderSessionState.{AccountPinned,QuotaClaimID}` persisted + restored so
  the pin survives restart/replay.
- `provider_registry.go` — `newAdapterForAccount` seam (preferred factory;
  "" resolves active as before); `AdapterForAccount` fails closed when a pin
  targets a provider without the seam; `Selectable`/`AdapterWithScope` updated.
  All live factories migrated: codex app-server, claude, gemini, grok,
  opencode, devin now resolve via `resolveAdapterAccount` — strict pin
  matching (a stale pin errors rather than silently binding another account).
- `provider_accounts.go` — `resolveAdapterAccount` helper (""/"default" =
  ambient, real id = strict pin).
- `opencode_model_variants.go` / `devin_adapter.go` —
  `opencodeLaunchEnvForAccount` / `devinLaunchEnvForAccount` (zero-arg
  wrappers preserved for the variants prober and prewarm).
- `cli/root.go` — `SetQuotaTelemetry` bridges `loadAccountLaunchMetadata`
  (cli-owned per-provider quota probes) into the runner seam; probe failure
  yields unknown headroom (manual-only).

## Lock ordering

`s.mu → quotaMu` is the only permitted nesting (provider-limit emit records
blocks under it). Claims write the ledger under `quotaMu`, release it, then
take `s.mu` for the run pin — never the reverse.

## Why

CP-87 P-3/P-4: routing must decide per leg, durably, without touching the
global account selector — a pinned leg may legitimately diverge from the
active account, and restart must replay the same binding. Cooldown +
auto-cap bound IP-safety risk when several accounts share one egress IP.

## Tests

`task447_quota_preflight_test.go` — 10 red-first tests: hub/child demand
resolution, headroom ranking, unknown-quota manual-only, exact-20s cooldown
(block before, eligible at deadline), stable cooldown timestamps across
refresh, max-2 auto cap, no global SetActiveAccount mutation, concurrent
claim serialization (exactly one winner), restart persistence of
claim + cooldown.

## Blast radius (gitnexus impact, pre-edit)

- `spawnChildRun` CRITICAL (18 impacted / 12 direct / 9 processes) — mitigated:
  additive input field + inherit-only-when-pinned semantics; legacy path
  unchanged when no pin exists.
- `AdapterWithScope` LOW — now delegates to `AdapterForAccount("")`;
  `newAdapterForAccount` is additive; legacy factory fields retained for
  fakes/placeholders.
- `createRun`, `ProviderSessionState`, admission guard — additive fields;
  pinned-vs-unpinned guard preserves the pre-447 `provider_account_changed`
  contract for unpinned runs.
