# Task-210: Grok Account Model — Detect, Connect, Switch, Quota

## Metadata

- Document ID: `Task-210`
- Title: `Grok Account Model — Detect, Connect, Switch, Quota`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/todo/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [Task-207: Grok Controlled Adapter MVP (Chat/Stream/Resume)](./Task-207-Grok-Controlled-Adapter-MVP.md)
- Child Documents: `None`
- Related Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [Task-018: Auto Switch Account](../done/Task-018-Auto-Switch-Account.md), [Task-059: Desktop Check Version Tested Baseline Config](../done/Task-059-Desktop-Check-Version-Tested-Baseline-Config.md), [Task-080: Claude Account Quota Usage Fetch](../done/Task-080-Claude-Account-Quota-Usage-Fetch.md), [BUG-092: History Resume Fails When Persisted Provider Account ID Is Stale](../done/BUG-092-History-Resume-Fails-When-Persisted-Provider-Account-ID-Is-Stale.md)
- Replaces: `None`
- Tags: `grok, grok-build, accounts, grok-home, detection, quota, multi-account`

## AI Quick View

### Summary

- Add Grok to every hard-coded `codex/claude/gemini` provider switch in the account subsystem so multiple Grok accounts isolate cleanly by `GROK_HOME`, exactly mirroring Codex's `CODEX_HOME` model (slot 0 = `~/.grok`, slots 1..N = managed `~/.grokHomeN`).
- Wire CLI detection (`grok --version`, currently `0.2.93`) into `providerSpecs()`/`detectProvider()` and add a `CompatTestedGrokVersion` baseline check.
- Wire `grok login` / `grok login --device-auth` as the connect flow, and classify the live-observed `402 personal-team-blocked:spending-limit` signal as a terminal, non-retryable usage-limit error with switch/reset guidance (not a login failure).

### Current Ask

- Make `flowpilot providers list --json` report Grok's install/version/auth state; make "connect account" create an isolated `GROK_HOME`; make account switch work exactly like Codex/Claude (lazy per-turn resolve + in-flight turn interrupt); make the account card show whatever Grok exposes for email/plan/models, and classify quota exhaustion correctly.

### Key Decisions

- `T-1` No `ProviderAccount` struct change, no `provider-accounts.json` schema change, no Supabase migration — the account/session persistence layer is already provider-generic (verified by research: `provider_key` is opaque text everywhere it's stored).
- `T-2` Follow the Codex pattern exactly for home isolation: `managedProviderHomePrefix("grok")→".grokHome"`, `NextAccountHomePath` prefix, `DiscoverProviderAccountHomes`+`discoverGrokAccountHomes`+`isValidGrokAccountPath` (looks for `config.toml`/`auth.json`), `defaultAuthCandidates`/`accountAuthPaths`/`hasValidProviderAuthFile` entries for `~/.grok/auth.json`.
- `T-3` `getEnvForExecution` and `providerEnvSetCommand` must set/export `GROK_HOME` and strip it from any unrelated inherited env, exactly like the `CODEX_HOME` case.
- `T-4` Connect uses `StartInteractiveAuth`/`AuthenticateProvider` → `grok login` (interactive) with a `grok login --device-auth` variant available for headless/managed-home flows.
- `T-5` Quota display (`loadAccountLaunchMetadata`→`loadGrokAccountMetadata`) shows whatever is honestly available (email/plan from `~/.grok/auth.json`; only show usage bars if a machine-readable source is found) — do not fabricate a usage percentage from the turn-time `402` signal alone.
- `T-6` The `402 personal-team-blocked:spending-limit` signal (and any exit-code equivalent) must be classified in `isProviderUsageLimitError`/adapter error mapping as terminal + non-retryable, with the same "switch account or wait" UX Claude's usage-limit path already has.

### Constraints

- **PLUGIN-ONLY / ZERO BASE REGRESSION (CP-46 `P-0`):** additive-only across all ~15 provider switches; append the `grok` case, never reorder existing cases; `getEnvForExecution` strip-list add of `GROK_HOME` must not disturb `CODEX_HOME`/`GEMINI_HOME`/`HOME` handling; `compat.go` fields are appended (never reordered/renamed) so `CheckVersionSettings` stays valid for Codex/Claude.
- Do not change `ProviderAccount`, `provider-accounts.json` load/save, `ActivateProviderAccount`, `SetActiveAccount`, `deterministicProviderAccountID`, or `ResolveProviderAccount` — all are already provider-generic per research; only add `grok` branches to switches that enumerate providers.
- Do not regress Codex/Claude/Gemini account discovery, connect, switch, or quota behavior.
- Follow the deterministic-ID discipline (`sha256(providerKey+":"+homePath)`) for any auto-discovered/managed Grok account so stored session references survive `provider-accounts.json` regeneration (the exact bug class `BUG-092`..`095` fixed for other providers).

### Open Questions

- `Q-1` (CP-46 `Q-5`) Does Grok expose any machine-readable quota/usage endpoint beyond the turn-time `402`, or is the account card limited to email/plan?
- `Q-2` (CP-46 `Q-6`) Does `grok login --device-auth` cleanly isolate to a fresh `GROK_HOME` the same way Codex's device flow does, with no cross-contamination of a previously-logged-in default home?

### Source Refs

- `CP-46` section `P-8`; parity rows `GR-12`, `GR-14`, `GR-18`, `GR-19`, `GR-20`, `GR-25`; Risks `R-5`, `R-7`.
- `apps/local-runner/internal/runner/provider_accounts.go` (`managedProviderHomePrefix`, `DiscoverProviderAccountHomes`, `syncManagedProviderAccounts`, `deterministicProviderAccountID`), `runner.go` (`providerSpecs`, `NextAccountHomePath`, `defaultAuthCandidates`, `accountAuthPaths`, `hasValidProviderAuthFile`, `getEnvForExecution`, `providerEnvSetCommand`, `StartInteractiveAuth`, `AuthenticateProvider`, `providerAuthStatus`), `provider_account_terminal.go` (`loadAccountLaunchMetadata`, `loadClaudeQuota` as the pattern), `interactive_service.go` (`isProviderUsageLimitError`), `compat.go` (`CompatTestedClaudeVersion`/`CompatTestedCodexVersion` pattern), `cli/root.go` (`/provider-accounts/defaults`, `/context` provider loops).

## 1. Goal

Make Grok a fully account-manageable provider: detected with version, connectable with an isolated home, switchable across multiple accounts with deterministic IDs, and honest about quota/usage — with zero changes to the already-generic account/session persistence layer.

## 2. Parent Links

- coding plan: `CP-46`
- tech design: `SD-06`, `SD-14`
- system spec: `SS-05`
- specific upstream ids: `CP-46 P-8`

## 3. Trigger

Task-207 makes Grok run turns for one account; this task makes Grok manageable the way Codex/Claude/Gemini already are — detection, multiple isolated accounts, switching, and quota-failure classification.

## 4. Exact Change

- `T-1` `runner.go` `providerSpecs()`: add `{Key:"grok", Label:"Grok", BinaryName:"grok", InstallHint:"irm https://x.ai/cli/install.ps1 | iex (Windows) / curl -fsSL https://x.ai/cli/install.sh | sh (mac/linux)", Models:[...]}`.
- `T-2` `runner.go` `providerAuthStatus`/`providerInstallCommand`: add `case "grok"`.
- `T-3` `provider_accounts.go` `managedProviderHomePrefix`: `grok→".grokHome"`; `runner.go` `NextAccountHomePath`: same prefix.
- `T-4` `provider_accounts.go` `DiscoverProviderAccountHomes` switch + new `discoverGrokAccountHomes()` (checks `GROK_HOME` env, default `~/.grok`, managed `~/.grokHomeN` via `discoverManagedProviderHomeSlots`) + `isValidGrokAccountPath()` (looks for `config.toml`/`auth.json`); extend the hard-coded `syncManagedProviderAccounts`/`syncProviderAccounts` provider loop to include `"grok"`.
- `T-5` `runner.go` `defaultAuthCandidates` + `accountAuthPaths` + `hasValidProviderAuthFile`: add Grok home `~/.grok`, auth file `~/.grok/auth.json`, and a content-sniff validator.
- `T-6` `runner.go` `getEnvForExecution`: add `case "grok": set GROK_HOME=home`; add `GROK_HOME` to the strip-list so it never leaks from the parent process. `providerEnvSetCommand`: add grok export branch (posix + windows).
- `T-7` `runner.go` `StartInteractiveAuth`: `case "grok": authCommand = "<bin> login"` (+ expose a device-auth variant for `ConnectProviderAccount`, e.g. a flag/param routing to `"<bin> login --device-auth"`). `AuthenticateProvider`: `grok → "grok login"`.
- `T-8` `cli/root.go`: add `"grok"` to the hard-coded `[]string{"codex","claude","gemini"}` provider loops at `/provider-accounts/defaults` and `/context`.
- `T-9` `provider_account_terminal.go` `loadAccountLaunchMetadata` switch + new `loadGrokAccountMetadata(home)` reading `~/.grok/auth.json` for email/plan; wire into `loadAccountLaunchMetadata` dispatch.
- `T-10` `interactive_service.go` `isProviderUsageLimitError` (+ any Grok-adapter-level error mapping): recognize the `402`/`personal-team-blocked`/`spending-limit` signature and classify it terminal/non-retryable with actionable text.
- `T-11` `compat.go`: add `CompatTestedGrokVersion` (seed `"0.2.93"`), extend `CompatConfig`/`CompatVersionInfo`/`CompatLoadInfo`/`RunCompatCheck` + `compatRunVersion(ctx,"grok")`.
- `T-12` Tests: account discovery for default/`GROK_HOME`/managed-slot homes with deterministic IDs; connect creates an isolated home + flips `AuthStatus`; switch interrupts in-flight turns and the next turn resolves the new home; quota-error classification test using the captured `402` signature.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/provider_accounts.go`, `runner.go`, `provider_account_terminal.go`, `interactive_service.go`, `compat.go`, `apps/local-runner/internal/cli/root.go`
- modules: provider-account discovery/isolation, interactive-service error classification, compat/version-baseline check
- routes: existing `/providers`, `/provider-accounts/*`, `/client/provider-accounts` (no new routes, just new provider coverage)
- tables: none (provider-accounts.json + Supabase tables already provider-generic)

## 6. Acceptance Check

- `flowpilot providers list --json` reports Grok installed with version `0.2.93`+ and a compat classification vs `CompatTestedGrokVersion`.
- Connecting a new Grok account creates an isolated `~/.grokHomeN` directory and a deterministic-or-random `ProviderAccount` row with `AuthStatus` correctly reflecting login state.
- Switching the active Grok account interrupts any in-flight Grok turn recoverably and the next turn runs under the new home's env.
- A simulated `402 spending-limit` response is classified as a terminal usage-limit error, not a login failure, and not retried.

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` Detection + version baseline work for Grok end to end via `flowpilot providers list/detect`.
- [ ] `DOD-2` Multiple Grok accounts isolate correctly by `GROK_HOME` / `~/.grokHomeN`, with deterministic IDs for auto-discovered/managed slots.
- [ ] `DOD-3` Connect (`grok login`, and a device-auth variant) works and flips account status to connected.
- [ ] `DOD-4` Switch reuses existing `ActivateProviderAccount`/`SetActiveAccount` mechanics with no new code path; in-flight turns interrupt recoverably.
- [ ] `DOD-5` Account metadata (email/plan/models) renders through the generic `ProviderAccountSummary` shape with no schema change.
- [ ] `DOD-6` `402`/spending-limit is classified as terminal usage-limit, not login/retryable.
- [ ] `DOD-7` **Base-regression (`P-0`):** every `grok` branch in the ~15 shared switches (`providerSpecs`, `managedProviderHomePrefix`, `NextAccountHomePath`, `getEnvForExecution` + strip-list, `providerEnvSetCommand`, `defaultAuthCandidates`, `accountAuthPaths`, `hasValidProviderAuthFile`, `isProviderUsageLimitError`, `compat.go` structs, `cli/root.go` loops) is appended without reordering; a per-function test asserts codex/claude/gemini inputs return byte-identical values; `getEnvForExecution` still strips `CODEX_HOME`/`GEMINI_HOME`/`HOME` exactly as before.
- [ ] `DOD-8` Codex/Claude/Gemini account discovery, connect, switch, and quota tests remain green and unchanged.

## 7. Out of Scope

- Desktop rendering of the new provider/account (Task-211).
- MCP credential handling for Grok's `~/.grok/mcp_credentials.json` beyond what Task-209 already requires.
- Grok-as-handoff-source transcript extraction (Task-212).

## 8. Completion Notes

- result:
- implementation notes:
- verification:
- follow-ups:
- upstream docs updated:
