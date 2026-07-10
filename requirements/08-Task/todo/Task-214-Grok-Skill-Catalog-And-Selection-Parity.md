# Task-214: Grok Skill-Catalog And Selection Parity

## Metadata

- Document ID: `Task-214`
- Title: `Grok Skill-Catalog And Selection Parity`
- Phase: `task`
- Status: `draft`
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
- `skillpack.Install` (`apps/local-runner/internal/skillpack/install.go:138-179`, invoked on project bind from `engine_setup.go:289`) is **provider-agnostic**: it always writes FlowPilot's built-in `SKILL.md` files into both `<repo>/.claude/skills/` and `<repo>/.agents/skills/` for every bound project, regardless of which provider the project uses. Because Grok natively scans both of those directories, **FlowPilot's built-in skills already reach Grok's own automatic invocation with zero Grok-specific code** — this part needs no fix.
- What is actually missing is FlowPilot's own **skill-selection feature** (the `SelectedSkills`/`injectSelectedSkills` picker used by chat/workflow steps to force-inject one specific skill's content into a turn's prompt) — a separate mechanism from Grok's native auto-invocation. Three small gaps, each additive:
  1. `ProviderCapabilities.SkillSelection` is unset (`false`) for Grok's registration (`provider_registry.go:418-420`), even though `promptPrep` already calls `r.injectSelectedSkills(...)` for Grok (`provider_registry.go:456-461`) — that wiring is currently dead because the UI/flow that populates `TurnRequest.SelectedSkills` gates on this capability flag.
  2. `interactive_catalog.go`'s `discoverProjectSkills`/`providerHomeSkillDirs` switches (lines 149-156, 194-213) only branch on `codex`/`gemini`/`claude` — there is no `grok` case, so FlowPilot's skill-catalog/picker never lists Grok's project-local (`.agents/skills`) or provider-home (`~/.grok/skills`) skills as selectable options.
  3. `skillpack.providerStatuses` (`install.go:32-36`) — the table `Status()` reads to report per-provider install status — has no `grok` row, so a Settings-style "is this skill installed for Grok" query would silently omit Grok even though the file is actually installed at the shared `.agents/skills` path.

### Current Ask

- Close the three FlowPilot-side gaps above so Grok has the same skill-*selection* surface Claude/Codex/Gemini already have, on top of the skill-*auto-invocation* parity that already works today via Grok's native `.agents/skills` scanning.

### Key Decisions

- `T-1` **No new writer, no new directory.** `skillpack.Install`'s `installRoots` (`.claude/skills`, `.agents/skills`) stay unchanged — they are provider-agnostic and Grok already reads `.agents/skills` natively per its own docs. Do **not** add a third `.grok/skills` install target; that would duplicate files for no behavioral gain and add a second thing to keep in sync.
- `T-2` **Set `SkillSelection: true` on Grok's `ProviderCapabilities`** (`provider_registry.go:418-420`) — the minimal change that turns the already-existing `promptPrep`/`injectSelectedSkills` wiring live. Verify this doesn't implicitly change any other capability-gated UI path (grep every read of `ProviderCapabilities.SkillSelection` before flipping the flag).
- `T-3` **Add a `grok` case to `discoverProjectSkills`** (`interactive_catalog.go:149-156`) mapping to `.agents/skills` with `Source:"flowpilot"` — same directory Codex/Gemini already use (`T-1`: no new directory), so this is a pure additive `case "codex", "gemini", "grok":` widen, not a new code path.
- `T-4` **Add a `grok` case to `providerHomeSkillDirs`** (`interactive_catalog.go:194-213`) returning `[homePath/.grok/skills, homePath/skills]`, mirroring the Claude/Gemini two-path pattern (compat dir first, bare `skills` dir second) — `homePath` here is the resolved `GROK_HOME`/account home, not literally `~`, so this correctly respects multi-account isolation (`DiscoverProviderAccountHomes` already resolves per-account homes for every other provider this function serves).
- `T-5` **Add a `grok` row to `skillpack.providerStatuses`** (`install.go:32-36`) pointing at `.agents/skills` (matching Codex/Gemini's row — the actual on-disk path is unchanged, this only widens what `Status()` reports).
- `T-6` **Base-regression (`P-0`):** every change above is a `switch`/slice widen or a single struct-literal field addition — no existing `case`/entry for `codex`/`claude`/`gemini` is reordered, removed, or altered. Codex/Claude/Gemini skill discovery, install-status reporting, and capability-gated skill-selection UI must return byte-identical results for those three providers before and after.

### Constraints

- Do not touch `skillpack.Install`'s `installRoots` or `Install()`'s write logic — the writer is correct and provider-agnostic already (`T-1`).
- Do not disable or alter `GROK_CLAUDE_MCPS_ENABLED`/`GROK_CURSOR_MCPS_ENABLED` (unrelated to skills) or touch the separate `GROK_CLAUDE_SKILLS_ENABLED`/`GROK_CURSOR_SKILLS_ENABLED` compat flags — those are the account owner's/Grok's own config surface, not FlowPilot's to manage (mirrors the `permission_mode` "never clobber user config" precedent from Task-208).
- `gemini_acp_transport.go` and any Codex/Claude-only file stay untouched (CP-46 `P-0`).

### Open Questions

- `Q-1` Should FlowPilot's skill catalog also surface Grok's own **bundled** skills (`check-work`, `docx`, `xlsx`, `pptx`, etc., extracted to `~/.grok/skills/` by Grok itself on startup) in the picker, or only FlowPilot-authored ones? `providerHomeSkillDirs`'s `homePath/skills` root (`T-4`) would pick these up automatically since Grok's bundled skills live in the same directory as user-authored ones — decide whether that's desired or noisy, and filter by a known-bundled-name denylist if not.
- `Q-2` Grok's own `[skills] paths/ignore/disabled` config in `config.toml` (see live-captured `08-skills.md`) can add/hide skills FlowPilot never wrote. Should the picker reflect that config, or only show what FlowPilot itself can account for? Deferred — out of scope for this pass.

### Source Refs

- Live evidence: `apps/local-runner/internal/runner/testdata/grok_acp/live_probe_raw.txt:27-28` (`skills-reload` ACP frame); real-machine `~/.grok/skills/*/SKILL.md`; real-machine `~/.grok/docs/user-guide/08-skills.md` (scan-order + `.agents/skills`/`.claude/skills` cross-scan default).
- Writer (unchanged): `apps/local-runner/internal/skillpack/install.go:27-30,138-179` (`installRoots`, `Install`); invoked from `apps/local-runner/internal/runner/engine_setup.go:289` (bind trigger, `engineInitTriggerBind`).
- Status reporting (gap): `apps/local-runner/internal/skillpack/install.go:32-36` (`providerStatuses`).
- Catalog/picker (gap): `apps/local-runner/internal/runner/interactive_catalog.go:105-213` (`listSkills`, `discoverProjectSkills`, `discoverProviderHomeSkills`, `providerHomeSkillDirs`).
- Capability + prompt injection (gap + existing dead wiring): `apps/local-runner/internal/runner/provider_registry.go:418-420` (Grok `Capabilities`), `:456-461` (Grok `promptPrep` already calling `r.injectSelectedSkills`); compare Claude/Codex/Gemini registrations at lines ~199, ~238, ~275, ~355 (`SkillSelection: true`).
- Per-turn injection (unchanged): `apps/local-runner/internal/runner/runner.go:1160` (`injectSelectedSkills`), `:1234` (`listSkillsInWorkspace`).

## 1. Goal

Give Grok the same FlowPilot skill-*selection* surface (catalog listing + capability-gated per-turn injection) that Claude/Codex/Gemini already have, without duplicating or touching the skill-*installation* writer, which already works for Grok today because Grok natively scans the same `.agents/skills` directory FlowPilot already installs into on every project bind.

## 2. Parent Links

- coding plan: `CP-46` (trigger/context — re-verifying Grok live surfaced this gap)
- tech design: `SD-06`
- system spec: `SS-05`
- specific upstream ids: Task-209 (`DOD-1`/skill-adjacent MCP parity, related but distinct mechanism)

## 3. Trigger

While re-verifying the Grok adapter against a real logged-in account, inspecting `~/.grok/skills/` to answer "does Grok read `.grok/skills/`" surfaced that (a) Grok does, natively and dynamically, and (b) it also natively scans the exact `.agents/skills` directory FlowPilot's `skillpack.Install` already writes into for every project — but FlowPilot's own skill-selection catalog/capability code has never had a `grok` case added, so the picker and per-turn force-injection path are still Codex/Claude/Gemini-only.

## 4. Exact Change

- `T-1` `provider_registry.go:418-420`: add `SkillSelection: true` to Grok's `ProviderCapabilities` literal.
- `T-2` `interactive_catalog.go:149-156` (`discoverProjectSkills`): widen `case "codex", "gemini":` to `case "codex", "gemini", "grok":` (same `.agents/skills` target, `Source:"flowpilot"`).
- `T-3` `interactive_catalog.go:194-213` (`providerHomeSkillDirs`): add a `case "grok":` returning `[]string{filepath.Join(homePath, ".grok", "skills"), filepath.Join(homePath, "skills")}`.
- `T-4` `install.go:32-36` (`providerStatuses`): append `{Provider: "grok", RootPath: filepath.Join(".agents", "skills")}`.
- `T-5` Tests: a catalog test asserting `discoverProjectSkills("grok", cwd)` reads `.agents/skills` and `providerHomeSkillDirs("grok", home)` returns the two Grok-home paths; a capability test asserting Grok's `ProviderCapabilities.SkillSelection==true` and that Codex/Claude/Gemini/`SkillSelection` values are unchanged (base-regression); a `skillpack` test asserting `Status()` now includes a `grok` entry without altering the Codex/Claude/Gemini entries' paths/order.
- `T-6` Manual/live check (reuse the account already logged in): confirm the desktop skill picker (wherever `SelectedSkills` is populated from — likely the same UI Codex/Claude/Gemini use) now lists Grok skills, and that selecting one actually changes the prompt `injectSelectedSkills` builds for a real Grok turn.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/provider_registry.go`, `apps/local-runner/internal/runner/interactive_catalog.go`, `apps/local-runner/internal/skillpack/install.go`.
- modules: skill catalog/discovery, skill-install status reporting, Grok provider capability declaration.
- routes: none new — reuses whatever endpoint already serves `listSkills`/`ProviderCapabilities` for Codex/Claude/Gemini.
- tables: none.

## 6. Acceptance Check

- Grok's `ProviderCapabilities.SkillSelection` is `true`; Codex/Claude/Gemini values are unchanged.
- `discoverProjectSkills("grok", cwd)` and `discoverProviderHomeSkills("grok")` return non-nil results when `.agents/skills` / `~/.grok/skills` (or the account's `GROK_HOME`) actually contain `SKILL.md` files; Codex/Claude/Gemini discovery is unchanged (byte-identical outputs for the same fixtures as before).
- `skillpack.Status(...)` includes a `grok` provider entry; Codex/Claude/Gemini entries are unchanged.
- A live turn with a `SelectedSkills` entry set actually shows the selected skill's content in the prompt Grok receives (verified via the existing live-account harness, `FLOWPILOT_LIVE_GROK=1`).
- Full existing suite passes unchanged (same pre-existing baseline failures, zero new ones).

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` Grok's `ProviderCapabilities.SkillSelection` is `true`; a base-regression test proves Codex/Claude/Gemini capability values are byte-identical to before.
- [ ] `DOD-2` `discoverProjectSkills`/`providerHomeSkillDirs` have a `grok` case; a fixture-based test proves both project-local (`.agents/skills`) and provider-home (`~/.grok/skills`-shaped) discovery work; Codex/Claude/Gemini discovery is unchanged.
- [ ] `DOD-3` `skillpack.providerStatuses` includes `grok`; a test proves `Status()` reports a Grok row without altering Codex/Claude/Gemini rows.
- [ ] `DOD-4` A live check (real account, `FLOWPILOT_LIVE_GROK=1`) proves a `SelectedSkills` entry actually reaches the prompt Grok receives for a real turn.
- [ ] `DOD-5` **Base-regression (`P-0`):** full existing suite passes unchanged (identical pre-existing baseline failures); no Codex/Claude/Gemini-only file is touched; `gemini_acp_transport.go` diff stays empty.

## 7. Out of Scope

- Any change to `skillpack.Install`'s writer or `installRoots` (`T-1` decision — not needed).
- Surfacing Grok's own bundled/plugin skills or its `config.toml [skills]` overrides in FlowPilot's picker (`Q-1`/`Q-2`, deferred).
- A native `.grok/skills` install target (deliberately rejected — `.agents/skills` already covers it).

## 8. Completion Notes

- result:
- implementation notes:
- verification:
- follow-ups:
- upstream docs updated:
