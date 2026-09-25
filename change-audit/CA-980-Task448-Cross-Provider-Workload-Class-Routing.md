# CA-980 — Task-448: cross-provider workload-class routing

## Summary

When no same-provider account qualifies (Task-447), the router can now
enumerate *other* registered providers and rank (provider, model, account)
candidates against the demand's workload class, required capabilities, and
context-window needs — deterministically, from configured provider priority +
normalized headroom. The resolver is **pure**: no claims, no leg minting, no
store mutation, no provider names in code. Quality mapping is user/catalog
data; adding a provider requires config, not Go changes.

## What changed

- `runner/quota_candidates.go` (new) — `RouteCandidate`, `CandidateSet`
  (`Eligible`/`ManualOnly`/`Rejected`), `CrossProviderCandidates`,
  `RankRouteCandidates`, `modelBindingFor`, `missingRequiredCaps`.
  Filter pipeline per registered provider except the current one:
  provider-unavailable → missing required capability → context window too
  small → no connected account → per-account headroom normalization with the
  same blocked/claimed/already-tried/exhausted demotions as Task-447.
  `AutoEligible` = exact class binding + healthy/exact/fresh headroom + zero
  rejection reasons. Unknown/stale quota or missing bindings land in
  `ManualOnly` with machine-readable reasons — never silently dropped.
- `runner/quota_preflight.go` — `ExecutionDemand` gains `RequiredCaps`
  (hub = streaming; provider-backed flow nodes = streaming + approval events
  + MCP tool bridge) and `MinContextTokens` (zero disables the filter).
- `runner/task448_cross_provider_test.go` (new) — eight additive tests
  covering registry enumeration, class-binding model selection, missing
  binding gating, capability/context-window rejection, unknown-quota
  manual-only, deterministic priority tie-break, and the sixth-provider
  acceptance check (gemini on a test registry + binding → eligible, zero
  router code changes).
- `runner/task447_quota_preflight_test.go` — fixture fix: claude auth body
  now matches `claudeAuthFileLooksValid` (camelCase `accessToken`); added
  gemini auth path. `$HOME` isolation in `task448Service` so host credential
  dirs can't leak real accounts into candidate buckets.

## Decisions

- Score = `(len(priority) - priorityIdx) * 1000 + headroom%`; unlisted
  providers share weight 0 and fall back to registration-order tie-breaks —
  `ProviderPriority` config beats raw headroom deterministically.
- Missing model binding keeps the provider's *default* model but demotes to
  `ManualOnly` (`missing_model_binding`) — the manual gate can still pick it.
- Cross-provider has no same-provider cooldown (that window is scoped to
  IP-safety on a single provider's accounts).
- One candidate row per connected account — the rotation target is the
  (provider, model, account) triple, matching the claim ledger's granularity.

## Verification

- `go test -count=1 -run 'TestTask448_' ./internal/runner/` — 8/8 green.
- `go test -count=1 ./internal/runner/` — full package suite.
- detect_changes: GitNexus CLI exposes no `detect_changes` command (MCP-only,
  not configured here); equivalent staged-diff scope review performed —
  only `quota_candidates.go`, `quota_preflight.go` (2 new demand fields),
  and the two test files touched.

## Follow-ups

- Task-449 consumes `CandidateSet` at the Flow/Vibe quota gate; the auto path
  may act only on `AutoEligible` rows and must mint legs (never mutate the
  global active account).
- Task-450 renders `CandidateSet` verbatim for the manual table — rejection
  reasons are already machine-readable.
