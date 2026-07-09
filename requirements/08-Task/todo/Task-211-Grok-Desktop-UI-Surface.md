# Task-211: Grok Desktop UI Surface

## Metadata

- Document ID: `Task-211`
- Title: `Grok Desktop UI Surface`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/todo/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [Task-210: Grok Account Model — Detect, Connect, Switch, Quota](./Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md)
- Child Documents: `None`
- Related Documents: [Task-036: Desktop Provider Accounts Sidebar](../done/Task-036-Desktop-Provider-Accounts-Sidebar.md), [Task-038: Desktop Provider Settings Connect Account](../done/Task-038-Desktop-Provider-Settings-Connect-Account.md), [Task-039: Desktop Provider Modal Connect Account](../done/Task-039-Desktop-Provider-Modal-Connect-Account.md), [Task-063: Desktop Chat Provider Card Picker](../done/Task-063-Desktop-Chat-Provider-Card-Picker.md), [Task-059: Desktop Check Version Tested Baseline Config](../done/Task-059-Desktop-Check-Version-Tested-Baseline-Config.md)
- Replaces: `None`
- Tags: `grok, grok-build, desktop, settings, ui, accounts, provider-picker`

## AI Quick View

### Summary

- Add the `"grok"` literal to every hard-coded provider enum/switch in the desktop app so Grok renders in settings (detect/install), the accounts sidebar (current account, limits, models, switch), and the chat provider/model picker — the same set of ~15 call sites the account/runner backend work already makes reachable via `GET /providers` and `GET /client/provider-accounts`.
- No new backend endpoint, no RTK/Zustand slice redesign, and no new component type is required — the desktop is already fully data-driven from the runner's existing HTTP endpoints; this task is additive enum/label/icon/color/default-model coverage.

### Current Ask

- Make Grok visually and functionally indistinguishable in the UI from Codex/Claude/Gemini for: detection status, add-account, current-account card (email/plan/usage-if-available/models), account switch, and the chat provider chip + model dropdown.

### Key Decisions

- `T-1` Follow the exact "Grok desktop-UI work list" produced during CP-46 research — every listed file/line is a required edit, not optional polish.
- `T-2` `ProviderAccountSummary` needs no shape change — it is already provider-agnostic; Grok appears the moment the runner returns `providerKey:"grok"`.
- `T-3` `VISION_PROVIDERS` gets a Grok entry only if Task-207/CP-46 `Q-1` proves image attachment support; otherwise leave Grok out of that set (matches the `Vision=false` capability decision).
- `T-4` Add a `GrokIcon()` inline SVG next to the existing `CodexIcon`/`ClaudeIcon`/`GeminiIcon` and a `--grok-brand` CSS variable + `.provider-chip-grok` rules, consistent with the existing per-provider styling pattern.
- `T-5` Generalize (not just special-case) the currently gemini-only "Install AGY CLI" button/message in `AiProvidersSettings.tsx` if Grok also needs an in-app install action; otherwise Grok can rely on detection-only + `installHint` display like Codex/Claude.

### Constraints

- **PLUGIN-ONLY / ZERO BASE REGRESSION (CP-46 `P-0`):** additive enum/label/icon/color/branch additions only; the `AiProvidersSettings` install-button generalization must not break Gemini's affordance; Codex/Claude/Gemini UI stays visually and functionally unchanged.
- Do not introduce a new state-management pattern; this app uses a single Zustand store (`state/store.ts`), not RTK — follow that convention exactly.
- Do not change the `RunnerClient` interface or add a Grok-specific HTTP call; reuse `listLocalProviders`, `listProviderAccounts`, `connectProviderAccount`, `activateProviderAccount`.
- Keep `CheckVersionSettings.tsx` (tested-baseline UI) changes optional/parity-only — do not block this task on it if Task-210's `CompatTestedGrokVersion` plumbing isn't ready yet; add the row when it is.

### Open Questions

- Whether Grok needs an in-app "Install" button (like Gemini's AGY) or install-detection-only (like Codex/Claude) — depends on whether FlowPilot wants to shell out the `irm .../install.ps1` / `curl .../install.sh` commands itself.
- Whether `pickDefaultModel` should default to `grok-build` or a specific `grok-4.5` alias once Task-210's model list is finalized.

### Source Refs

- `CP-46` section `P-9`; parity rows `GR-20`, `GR-25`.
- CP-46 authoring research ("Grok desktop-UI work list"), items A–H, specifically:
  `apps/desktop-flowpilot/src/types/contract.ts:11` (`ProviderKey`), `packages/flowpilot-client-core/src/domain/adminModels.ts:89` (`SupportedModel.providerKey`), `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx` (rows 157/206/207), `apps/desktop-flowpilot/src/components/settings/CheckVersionSettings.tsx`, `apps/desktop-flowpilot/src/components/ProviderAccountsPanel.tsx` (lines 7-11, 66-72, 195), `apps/desktop-flowpilot/src/components/ChatInput.tsx` (lines 33, 58-62, 67), `apps/desktop-flowpilot/src/state/store.ts` (lines 83-98, 1772-1776), `packages/flowpilot-client-core/src/domain/adminLogic.ts:3-15`, `apps/desktop-flowpilot/src/styles.css` (lines 15-17, 3629-3646), `apps/desktop-flowpilot/src/components/AgentsPanel.tsx:417-419`, `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`.

## 1. Goal

Surface Grok in the desktop app's settings, account panel, and chat picker with the same fidelity as Codex/Claude/Gemini, using only additive enum/label/icon/color/default-model changes — no new data-flow architecture.

## 2. Parent Links

- coding plan: `CP-46`
- tech design: `SD-06`
- system spec: `SS-05`
- specific upstream ids: `CP-46 P-9`

## 3. Trigger

Task-210 makes the runner backend report and manage Grok accounts; without this task the desktop still hides Grok behind hard-coded provider literal unions.

## 4. Exact Change

- `T-1` `types/contract.ts:11` — add `| "grok"` to `ProviderKey`.
- `T-2` `packages/flowpilot-client-core/src/domain/adminModels.ts:89` — add `| "grok"` to `SupportedModel.providerKey`.
- `T-3` `components/settings/AiProvidersSettings.tsx` — generalize the gemini-only install button/message (line ~157/206) if Grok needs one; add `"grok"` to the Add-Supported-Model provider `<select>` (~line 207).
- `T-4` `components/ProviderAccountsPanel.tsx` — add `{key:"grok", label:"Grok"}` to `PROVIDERS` (lines 7-11); extend the `as "claude"|"codex"|"gemini"` cast (~line 195) to include `"grok"`; decide `compactUsageLines`'s Grok behavior (lines 66-72, default to "show all lines" unless Grok needs filtering).
- `T-5` `components/ChatInput.tsx` — add `GrokIcon()` next to `GeminiIcon` (~line 33); add `{value:"grok", label:"Grok", icon:<GrokIcon/>}` to `PROVIDER_CARDS` (~lines 58-62); add `"grok"` to `VISION_PROVIDERS` (~line 67) only if vision is proven.
- `T-6` `state/store.ts` — add a `grok` branch to `pickDefaultModel` (~lines 83-98); add `if (providerKey==="grok") return "Grok";` to `providerLabel` (~lines 1772-1776).
- `T-7` `packages/flowpilot-client-core/src/domain/adminLogic.ts:3-15` — add a `grok-` model-id prefix to `resolveProviderKeyForModel`.
- `T-8` `styles.css` — add `--grok-brand` (lines ~15-17) and `.provider-chip-grok`/`.provider-chip-grok.provider-chip-selected` rules (~lines 3629-3646).
- `T-9` `components/AgentsPanel.tsx:417-419` — add a Grok provider-override chip (optional, only if agents can target Grok per Task-209).
- `T-10` `components/settings/CheckVersionSettings.tsx` + `packages/flowpilot-client-core/src/domain/runner.ts:8-17` — add a third `VersionRow`/config input for Grok once `CompatTestedGrokVersion` exists (Task-210).
- `T-11` `client/MockRunnerClient.ts` — add a Grok entry to `MOCK_PROVIDER_ACCOUNTS` and provider branches so mock-mode renders Grok; check `state/store.test.ts` for provider-enum assumptions that need updating.
- `T-12` Verify provider-neutral chat surfaces render for Grok without new code: reasoning-effort control (if present in the composer) offers Grok's effort levels; the turn-skills summary chip (Task-053) and slash commands (Task-087) work for a Grok chat. Add coverage only where a hard-coded provider check blocks Grok.

## 5. Touched Areas

- files: `types/contract.ts`, `components/settings/AiProvidersSettings.tsx`, `components/settings/CheckVersionSettings.tsx`, `components/ProviderAccountsPanel.tsx`, `components/ChatInput.tsx`, `components/AgentsPanel.tsx`, `state/store.ts`, `client/MockRunnerClient.ts`, `styles.css`; `packages/flowpilot-client-core/src/domain/adminModels.ts`, `adminLogic.ts`, `runner.ts`
- modules: desktop settings, account panel, chat composer/picker
- routes: none (reuses existing `GET /providers`, `GET /client/provider-accounts`, `POST /provider-accounts/{connect,activate,test}`)
- tables: none

## 6. Acceptance Check

- With a Grok account connected, the settings page shows Grok installed + version; the account sidebar shows the Grok account card with email/plan and a "Set active"/switch control; the chat composer shows a Grok chip and a Grok model list with a sensible default.
- Mock mode renders Grok without crashing (`MockRunnerClient` coverage).
- No visual/functional regression for Codex/Claude/Gemini rows, chips, or cards.

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` `ProviderKey` and `SupportedModel.providerKey` include `"grok"`.
- [ ] `DOD-2` Settings page renders Grok detection/version and (if applicable) an install action.
- [ ] `DOD-3` Account sidebar groups, displays, and can switch a Grok account.
- [ ] `DOD-4` Chat picker shows a Grok chip/icon and a working model dropdown with a default model.
- [ ] `DOD-5` Brand color + chip styling exist for Grok.
- [ ] `DOD-6` Mock-mode and existing store tests pass with the new enum value present.
- [ ] `DOD-7` Reasoning-effort control (if present), turn-skills chip, and slash commands render for a Grok chat.
- [ ] `DOD-8` **Base-regression (`P-0`):** all desktop edits are additive enum/label/icon/color/branch additions; the `AiProvidersSettings` install-button generalization keeps Gemini's affordance working; Codex/Claude/Gemini chips, cards, settings rows, and pickers are visually and functionally unchanged.

## 7. Out of Scope

- Any new backend endpoint or data shape change (must be driven entirely by existing runner responses per Task-210).
- Tested-baseline UI (`CheckVersionSettings`) row if `CompatTestedGrokVersion` isn't ready — defer, don't block.
- In-app CLI installer implementation beyond the UI affordance decision (`Open Questions`).

## 8. Completion Notes

- result:
- implementation notes:
- verification:
- follow-ups:
- upstream docs updated:
