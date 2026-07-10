# Task-214: Grok Skill-Catalog And Selection Parity

## Metadata

- Document ID: `Task-214`
- Title: `Grok Skill-Catalog And Selection Parity`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- Child Documents: `None`
- Related Documents: [Task-209: Grok MCP, Ask-User, And Spawn-Agent Parity](../inprogress/Task-209-Grok-MCP-Ask-User-Spawn-Agent-Parity.md), [Task-213: Auto-Detect And Sync Provider Models Into The Supported-Models Catalog](../inprogress/Task-213-Auto-Detect-And-Sync-Provider-Models.md)
- Replaces: `None`
- Tags: `ai-providers, grok, skills, catalog, capabilities`

## AI Quick View

### Summary

- Live-verified (2026-07-10, real logged-in account): Grok's own CLI already natively discovers and auto-invokes skills. `~/.grok/skills/*/SKILL.md` exist on disk (bundled: `check-work`, `code-review`, `create-skill`, `docx`, `help`, `imagine`, `pptx`, `xlsx`); a live ACP capture shows the running process reload them on disk change (`{"id":"skills-reload","result":{"reloaded":1}}`, `testdata/grok_acp/live_probe_raw.txt:27-28`). Grok's own docs (`~/.grok/docs/user-guide/08-skills.md`) confirm the scan order `./.grok/skills` > `<repo_root>/.grok/skills` > `~/.grok/skills`, and — critically — that Grok **also scans `.agents/skills/` and `.claude/skills/` at every tier by default**, unless disabled via `GROK_CLAUDE_SKILLS_ENABLED`/`GROK_CURSOR_SKILLS_ENABLED` (a *different* flag pair than the `*_MCPS_ENABLED` ones the adapter already sets false in `grok_process.go`).
- `skillpack.Install` (`apps/local-runner/internal/skillpack/install.go:138-179`, invoked on project bind from `engine_setup.go:289`) writes FlowPilot's built-in `SKILL.md` files into every root in `installRoots` (currently `<repo>/.claude/skills/` and `<repo>/.agents/skills/`) for every bound project, regardless of which provider the project uses — the `Provider` field on each root is a label, not a gate. **Correction (per review): Grok is architecturally like Claude, not like Codex/Gemini** — it has its own dedicated, documented, prioritized skill directory (`.grok/skills/`), the same way Claude has `.claude/skills/`. Codex/Gemini have no such native directory of their own, which is why they piggyback on the generic `.agents/skills` convention. So Grok should get its **own install root** (`.grok/skills`), not just a ride on `.agents/skills` — even though the latter also works via Grok's cross-scan, the native, doc-recommended, git-shareable location is `.grok/skills`.
- What is actually missing is (a) a native `.grok/skills` install target, and (b) FlowPilot's own **skill-selection feature** (the `SelectedSkills`/`injectSelectedSkills` picker used by chat/workflow steps to force-inject one specific skill's content into a turn's prompt) — a separate mechanism from Grok's native auto-invocation. Four small gaps, each additive:
  1. `installRoots` (`install.go:27-30`) has no `.grok/skills` entry — FlowPilot's builtin skills never get physically installed to Grok's own native directory (they'd still be visible to Grok via its `.agents/skills` cross-scan, but not at the location Grok's own docs recommend for project/team-shared skills).
  2. `ProviderCapabilities.SkillSelection` is unset (`false`) for Grok's registration (`provider_registry.go:418-420`), even though `promptPrep` already calls `r.injectSelectedSkills(...)` for Grok (`provider_registry.go:456-461`) — that wiring is currently dead because the UI/flow that populates `TurnRequest.SelectedSkills` gates on this capability flag.
  3. `interactive_catalog.go`'s `discoverProjectSkills`/`providerHomeSkillDirs` switches (lines 149-156, 194-213) only branch on `codex`/`gemini`/`claude` — there is no `grok` case, so FlowPilot's skill-catalog/picker never lists Grok's project-local (`.grok/skills`) or provider-home (`<GROK_HOME>/skills`) skills as selectable options.
  4. `skillpack.providerStatuses` (`install.go:32-36`) — the table `Status()` reads to report per-provider install status — has no `grok` row, so a Settings-style "is this skill installed for Grok" query would silently omit Grok.

### Current Ask

- Give Grok its own native `.grok/skills` install target (mirroring Claude's `.claude/skills` treatment, not Codex/Gemini's shared `.agents/skills`), and close the three FlowPilot-side catalog/capability gaps so Grok has the same skill-*selection* surface Claude/Codex/Gemini already have.

### Key Decisions

- `T-1` **Grok gets its own install root, like Claude.** Add `{Provider: "grok", RootPath: filepath.Join(".grok", "skills")}` to `installRoots` (`install.go:27-30`). This mirrors how Claude gets a dedicated `.claude/skills` root instead of relying solely on `.agents/skills` — Grok has the same kind of first-class, documented, prioritized skill directory Claude does, so it earns the same treatment. `.agents/skills` keeps being written too (unchanged, still read by Codex/Gemini and cross-scanned by Grok) — no existing root is removed.
- `T-2` **Set `SkillSelection: true` on Grok's `ProviderCapabilities`** (`provider_registry.go:418-420`) — the minimal change that turns the already-existing `promptPrep`/`injectSelectedSkills` wiring live. Verify this doesn't implicitly change any other capability-gated UI path (grep every read of `ProviderCapabilities.SkillSelection` before flipping the flag).
- `T-3` **Add a `grok` case to `discoverProjectSkills`** (`interactive_catalog.go:149-156`) mapping to `.grok/skills` with `Source:"workspace"` — same treatment as Claude's `.claude/skills` case (a dedicated case, not a shared-`.agents` widen), since `T-1` now installs there.
- `T-4` **Add a `grok` case to `providerHomeSkillDirs`** (`interactive_catalog.go:194-213`) returning `[homePath/skills, homePath/.grok/skills]`, mirroring the **Codex** ordering (bare `skills` dir first, nested-compat dir second) rather than Claude's — because `homePath` here is the resolved account home (`GROK_HOME`, from `account.HomePath`) which, like `CODEX_HOME`, already points directly at the tool's own dot-directory (i.e. `GROK_HOME` *is* `~/.grok`, so skills live at `homePath/skills`, not `homePath/.grok/skills`). Claude's home-path convention differs (its `homePath` is a synthetic `$HOME` override that *contains* a `.claude` subfolder), which is why its case is ordered the other way.
- `T-5` **Add a `grok` row to `skillpack.providerStatuses`** (`install.go:32-36`) pointing at `.grok/skills` (matching the new `T-1` install root, not `.agents/skills`).
- `T-6` **Backfill existing projects.** `IsInstalled`'s sentinel list (`install.go:184-195`) gains a third sentinel (`.grok/skills/git-commit-format/SKILL.md`). Projects bound before this change will report `IsInstalled==false` once, which re-triggers `Install()` on the next bind check — self-healing the missing `.grok/skills` files (the two pre-existing roots are skipped via the version check, so this is a cheap no-op for them).
- `T-7` **Base-regression (`P-0`):** every change above is a `switch`/slice widen, an appended slice entry, or a single struct-literal field addition — no existing `case`/entry/root for `codex`/`claude`/`gemini` is reordered, removed, or altered. Codex/Claude/Gemini skill discovery, install-status reporting, install-writing, and capability-gated skill-selection UI must return byte-identical results/files for those three providers before and after.

### Constraints

- `installRoots`/`Install()` gains exactly one new entry (`T-1`) — no existing root's path, order, or write behavior changes; `.claude/skills` and `.agents/skills` keep being written for every project exactly as before.
- Do not disable or alter `GROK_CLAUDE_MCPS_ENABLED`/`GROK_CURSOR_MCPS_ENABLED` (unrelated to skills) or touch the separate `GROK_CLAUDE_SKILLS_ENABLED`/`GROK_CURSOR_SKILLS_ENABLED` compat flags — those are the account owner's/Grok's own config surface, not FlowPilot's to manage (mirrors the `permission_mode` "never clobber user config" precedent from Task-208).
- `gemini_acp_transport.go` and any Codex/Claude-only file stay untouched (CP-46 `P-0`).

### Open Questions

- `Q-1` Should FlowPilot's skill catalog also surface Grok's own **bundled** skills (`check-work`, `docx`, `xlsx`, `pptx`, etc., extracted to `~/.grok/skills/` by Grok itself on startup) in the picker, or only FlowPilot-authored ones? `providerHomeSkillDirs`'s `homePath/skills` root (`T-4`) would pick these up automatically since Grok's bundled skills live in the same directory as user-authored ones — decide whether that's desired or noisy, and filter by a known-bundled-name denylist if not.
- `Q-2` Grok's own `[skills] paths/ignore/disabled` config in `config.toml` (see live-captured `08-skills.md`) can add/hide skills FlowPilot never wrote. Should the picker reflect that config, or only show what FlowPilot itself can account for? Deferred — out of scope for this pass.

### Source Refs

- Live evidence: `apps/local-runner/internal/runner/testdata/grok_acp/live_probe_raw.txt:27-28` (`skills-reload` ACP frame); real-machine `~/.grok/skills/*/SKILL.md`; real-machine `~/.grok/docs/user-guide/08-skills.md` (scan-order: `./.grok/skills` > `<repo_root>/.grok/skills` > `~/.grok/skills`, plus `.agents/skills`/`.claude/skills` cross-scan default).
- Writer (gains one root, `T-1`): `apps/local-runner/internal/skillpack/install.go:27-30,138-179,184-195` (`installRoots`, `Install`, `IsInstalled`); invoked from `apps/local-runner/internal/runner/engine_setup.go:289` (bind trigger, `engineInitTriggerBind`).
- Status reporting (gap): `apps/local-runner/internal/skillpack/install.go:32-36` (`providerStatuses`).
- Catalog/picker (gap): `apps/local-runner/internal/runner/interactive_catalog.go:105-213` (`listSkills`, `discoverProjectSkills`, `discoverProviderHomeSkills`, `providerHomeSkillDirs`).
- Capability + prompt injection (gap + existing dead wiring): `apps/local-runner/internal/runner/provider_registry.go:418-420` (Grok `Capabilities`), `:456-461` (Grok `promptPrep` already calling `r.injectSelectedSkills`); compare Claude/Codex/Gemini registrations at lines ~199, ~238, ~275, ~355 (`SkillSelection: true`).
- Home-path convention proof: `apps/local-runner/internal/runner/provider_accounts.go:834-854` (`discoverGrokAccountHomes` returns `<homeDir>/.grok` directly, same shape as `discoverCodexAccountHomes`'s `~/.codex`).
- Per-turn injection (unchanged): `apps/local-runner/internal/runner/runner.go:1160` (`injectSelectedSkills`), `:1234` (`listSkillsInWorkspace`).

## 1. Goal

Give Grok its own native `.grok/skills` install root (matching how Claude gets `.claude/skills`, since Grok has the same kind of first-class, documented skill directory Claude does — unlike Codex/Gemini, which only have the shared `.agents/skills` convention), and give Grok the same FlowPilot skill-*selection* surface (catalog listing + capability-gated per-turn injection) that Claude/Codex/Gemini already have.

## 2. Parent Links

- coding plan: `CP-46` (trigger/context — re-verifying Grok live surfaced this gap)
- tech design: `SD-06`
- system spec: `SS-05`
- specific upstream ids: Task-209 (`DOD-1`/skill-adjacent MCP parity, related but distinct mechanism)

## 3. Trigger

While re-verifying the Grok adapter against a real logged-in account, inspecting `~/.grok/skills/` to answer "does Grok read `.grok/skills/`" surfaced that (a) Grok does, natively and dynamically, with `.grok/skills` as its own first-class, doc-recommended, git-shareable skill location (same tier structure as `.claude/skills` for Claude), and (b) FlowPilot's `skillpack.Install` had never been given a matching `.grok/skills` root, nor had the skill-selection catalog/capability code ever had a `grok` case added — so neither the native-directory install nor the picker/force-injection path existed for Grok.

## 4. Exact Change

- `T-1` `install.go:27-30` (`installRoots`): append `{Provider: "grok", RootPath: filepath.Join(".grok", "skills")}`.
- `T-2` `install.go:184-195` (`IsInstalled`): append a third sentinel, `filepath.Join(targetRepoDir, ".grok", "skills", "git-commit-format", "SKILL.md")`.
- `T-3` `install.go:32-36` (`providerStatuses`): append `{Provider: "grok", RootPath: filepath.Join(".grok", "skills")}`.
- `T-4` `provider_registry.go:418-420`: add `SkillSelection: true` to Grok's `ProviderCapabilities` literal.
- `T-5` `interactive_catalog.go:149-156` (`discoverProjectSkills`): add a dedicated `case "grok":` returning `providerSkillsFromDir(filepath.Join(cwd, ".grok", "skills"), "workspace")` (own case, not folded into the `codex, gemini` one — `.grok/skills` is a distinct path from `.agents/skills`).
- `T-6` `interactive_catalog.go:194-213` (`providerHomeSkillDirs`): add a `case "grok":` returning `[]string{filepath.Join(homePath, "skills"), filepath.Join(homePath, ".grok", "skills")}` (Codex ordering — `homePath` already *is* the `.grok` dir; see the home-path-convention proof in Source Refs).
- `T-7` Tests: a `skillpack` test asserting `Install` now writes `.grok/skills/<skill>/SKILL.md` alongside the two existing roots, and that `IsInstalled`/`Status()` include Grok without altering Codex/Claude/Gemini's entries/paths/order; a catalog test asserting `discoverProjectSkills("grok", cwd)` reads `.grok/skills` and `providerHomeSkillDirs("grok", home)` returns `[home/skills, home/.grok/skills]`; a capability test asserting Grok's `ProviderCapabilities.SkillSelection==true` and that Codex/Claude/Gemini `SkillSelection` values are unchanged (base-regression).
- `T-8` Manual/live check (reuse the account already logged in): confirm a real project bind now creates `.grok/skills/*/SKILL.md`; confirm the desktop skill picker now lists Grok skills; confirm selecting one actually changes the prompt `injectSelectedSkills` builds for a real Grok turn.

## 5. Touched Areas

- files: `apps/local-runner/internal/skillpack/install.go`, `apps/local-runner/internal/runner/provider_registry.go`, `apps/local-runner/internal/runner/interactive_catalog.go`.
- modules: skill install writer, skill install-status reporting, skill catalog/discovery, Grok provider capability declaration.
- routes: none new — reuses whatever endpoint already serves `listSkills`/`ProviderCapabilities`/bind-init for Codex/Claude/Gemini.
- tables: none.

## 6. Acceptance Check

- Binding a project creates `.grok/skills/<skill>/SKILL.md` for every built-in skill, alongside the existing `.claude/skills` and `.agents/skills` writes (unchanged).
- Grok's `ProviderCapabilities.SkillSelection` is `true`; Codex/Claude/Gemini values are unchanged.
- `discoverProjectSkills("grok", cwd)` and `discoverProviderHomeSkills("grok")` return non-nil results when `.grok/skills` (project or `GROK_HOME`-resolved) actually contains `SKILL.md` files; Codex/Claude/Gemini discovery is unchanged (byte-identical outputs for the same fixtures as before).
- `skillpack.Status(...)`/`IsInstalled(...)` include/require a `grok` entry; Codex/Claude/Gemini entries and existing `IsInstalled` behavior for already-installed pre-`T-1` projects are unchanged aside from the one-time self-heal (`T-6` in Key Decisions).
- A live turn with a `SelectedSkills` entry set actually shows the selected skill's content in the prompt Grok receives (verified via the existing live-account harness, `FLOWPILOT_LIVE_GROK=1`).
- Full existing suite passes unchanged (same pre-existing baseline failures, zero new ones).

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` `skillpack.Install` writes `.grok/skills/<skill>/SKILL.md` for every built-in skill on project bind, alongside the unchanged `.claude/skills`/`.agents/skills` writes; a test proves this without altering the other two roots' output. (`TestInstall_WritesGrokSkillsRoot`.)
- [x] `DOD-2` `IsInstalled` and `skillpack.providerStatuses`/`Status()` recognize the `.grok/skills` root; a test proves Codex/Claude/Gemini rows/paths are unchanged. (`TestIsInstalled_RequiresGrokSentinel`, `TestProviderStatuses_IncludesGrokWithoutAlteringOthers`.)
- [x] `DOD-3` Grok's `ProviderCapabilities.SkillSelection` is `true`; a base-regression test proves Codex/Claude/Gemini capability values are byte-identical to before. Set on both the authoritative instance method (`grokAdapter.Capabilities()`) and the static `ProviderRegistration` literal. (`TestGrokAdapterCapabilitiesMatchProvenSet`, updated.)
- [x] `DOD-4` `discoverProjectSkills`/`providerHomeSkillDirs` have a `grok` case pointed at `.grok/skills`; a fixture-based test proves both project-local and provider-home (`GROK_HOME`-shaped) discovery work; Codex/Claude/Gemini discovery is unchanged. (`interactive_catalog_test.go`: `TestDiscoverProjectSkillsGrokReadsDotGrokSkills`, `TestDiscoverProjectSkillsBaseRegression`, `TestProviderHomeSkillDirsGrokMirrorsCodexOrdering`, `TestProviderHomeSkillDirsBaseRegression`.)
- [x] `DOD-5` A live check (real account, `FLOWPILOT_LIVE_GROK=1`) proves a `SelectedSkills` entry actually reaches the prompt Grok receives for a real turn. (`TestLiveRealGrokSelectedSkillReachesThePrompt`: gave Grok a skill file mandating an exact reply marker; Grok called `read_file` on the referenced skill path and replied with the mandated marker — full round trip proven live, not just fixture-tested.)
- [x] `DOD-6` **Base-regression (`P-0`):** full existing suite passes unchanged (identical pre-existing baseline failures); no Codex/Claude/Gemini-only file is touched; `gemini_acp_transport.go` diff stays empty. (`go vet ./...` clean; full suite: 1296 passed / 15 failed / 18 skipped — the same 15 pre-existing, unrelated failures as the clean baseline verified earlier this session, zero new ones; `git diff --stat` on `gemini_acp_transport.go` is empty.)

## 7. Out of Scope

- Surfacing Grok's own bundled/plugin skills or its `config.toml [skills]` overrides in FlowPilot's picker (`Q-1`/`Q-2`, deferred).
- Removing or consolidating the `.agents/skills` write/read path for Grok — it stays as a secondary, cross-scanned location; only `.grok/skills` is promoted to a first-class FlowPilot-managed root.

## 8. Completion Notes

- result: All 6 DOD items done. Grok now has (a) its own native `.grok/skills` install root alongside `.claude/skills`/`.agents/skills`, (b) `SkillSelection: true` on both the live adapter and the static registration, and (c) `discoverProjectSkills`/`providerHomeSkillDirs` cases pointed at `.grok/skills`/`<GROK_HOME>/skills`.
- implementation notes: `providerHomeSkillDirs("grok", ...)` deliberately mirrors Codex's ordering (`homePath/skills` first), not Claude's, because `GROK_HOME` already points directly at the `.grok` directory itself (same as `CODEX_HOME`) — proven against the real `discoverGrokAccountHomes` implementation, not assumed.
- verification: `go build ./...` / `go vet ./...` clean; full suite 1296 passed / 15 failed (identical pre-existing baseline) / 18 skipped; live-verified end to end against the real logged-in account (`FLOWPILOT_LIVE_GROK=1`) — a real Grok turn read a selected skill file via `read_file` and followed its instruction exactly.
- follow-ups: `Q-1`/`Q-2` (surfacing Grok's own bundled/plugin skills and `config.toml [skills]` overrides in FlowPilot's picker) remain open, deferred.
- upstream docs updated: CP-46 §10.2 not yet touched for this task (skills weren't part of its original DOD table) — no update needed there; this doc is the record of record.
