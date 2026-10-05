# CA-1218 — spawn_agent provider override routed child to unconnected provider (live run-295277)

## Evidence

Live `vibe-adopt` run `run-295277` (provider `devin`, workspace
`PrivateVault-cp05`) wedged on `preflight_contract_plan = FAILED` with all
other sprint nodes pending:

1. The devin hub leg called `spawn_agent` for `contract-planner` with an
   explicit `provider: "codex"` — picked straight from the tool-schema
   example string `"Override provider key (codex, claude). Omit to inherit
   parent."`
2. `parseSpawnAgentInput` passes `args["provider"]` verbatim into
   `SpawnAgentInput.Provider`; `spawnChildRun` resolves `providerKey=codex`
   and creates child `run-295289`.
3. The child died at adapter construction: `no connected local account
   found for provider "codex"` — no Codex account exists on the machine
   (only devin + grok are connected).
4. The hub then had to burn a turn reporting the infra failure via
   `submit_review_outcome` (blocked), parking the flow `awaiting_user`.
   Cost: one dead run row, one FAILED node verdict, one recovery cycle —
   all for a failure that was knowable at spawn time.

First CP05 run (`run-295244`) hit the identical wall via the same override
channel before this run was relaunched on devin.

## Root cause

`spawn_agent`'s `provider` argument is a trust-me override: it is the only
input that can route a child to a provider the hub merely *imagined* —
parent inheritance and pack `model:` config are both operator-controlled,
but `in.Provider` comes from tool-call args with no validation against the
local account store. Nothing checked connectivity until adapter
construction deep inside the child turn.

## Fix

`spawnChildRun` now fail-closes when `in.Provider` is explicitly set and
the resolved provider has no connected local account:

- New `InteractiveService.ensureSpawnProviderConnected(providerKey)`
  (quota_claim.go) lists accounts via `listProviderAccounts()` and returns
  an error naming the unconnected provider **plus the connected set**
  (`connected: codex, devin, …`) so the hub can re-pick immediately or omit
  the override to inherit.
- The gate sits after providerKey resolution (model > agentDef > parent)
  and before `createRun`, so no dead child row is left behind.
- Scoped to the **cross-provider** explicit override: `in.Provider` reaches
  this path solely via tool-call/API args (engine dispatch never sets it),
  and a same-provider pin can never strand on accounts — the parent already
  runs under it. Pack-configured model routing and plain inheritance keep
  legacy behavior. Verified: `TestSpawnInheritSkipsProviderAccountGate`
  spawns with zero connected accounts through the inherit path, and
  `TestSpawnProviderOverrideConnectedAllowed` (same-provider) is ungated.

## Regression coverage

`ca1218_spawn_provider_account_gate_test.go` (3 tests):

- `TestSpawnProviderOverrideUnconnectedFailsClosed` — claude override with
  only codex connected → actionable error, no child row created.
  RED-verified: without the gate the same spawn produced a child that died
  at adapter construction (`provider_unavailable`).
- `TestSpawnProviderOverrideConnectedAllowed` — codex override with codex
  connected → child spawns normally.
- `TestSpawnInheritSkipsProviderAccountGate` — no connected accounts at
  all → inherit-path spawn still works (gate skipped).

Test fixtures isolate `HOME`/`USERPROFILE`/`APPDATA` +
`FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH` so the account sync sees only
planted credentials (`~/.codex/auth.json` with `"id_token"`), never the
real machine's devin/grok accounts.

## Files

- `internal/runner/interactive_service.go` — gate in `spawnChildRun`
- `internal/runner/quota_claim.go` — `ensureSpawnProviderConnected` helper
- `internal/runner/ca1218_spawn_provider_account_gate_test.go` — new

## Note

The tool-schema description still shows `(codex, claude)` as example
providers — a follow-up could make the description dynamically list only
connected providers, removing the hallucination source at the prompt
layer. Not required for correctness after this gate.
