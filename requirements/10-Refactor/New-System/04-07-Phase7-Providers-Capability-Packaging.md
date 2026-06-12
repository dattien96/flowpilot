# 04-07 — Phase 7: Provider Placeholders, Capability UI & Packaging

> Part of the `04` coding plan. Final phase: keep the provider abstraction honest
> and ship the desktop app.

## Goal

Ensure Claude/Gemini are first-class (if disabled) in the abstraction, surface
provider capabilities in the UI, and package/sign the desktop app for distribution.

## Claude & Gemini placeholders

- Each implements the same provider contract, returning
  `UnsupportedProviderRuntimeError` until built. They appear in the registry,
  expose capabilities as disabled/unknown, cannot be selected for controlled runs
  unless enabled, and surface clearly in Admin Web + Desktop.
- **Claude future:** Claude Agent SDK (preferred — lifecycle, tool, approval
  control) or `claude -p`. Must normalize session id, streamed text, tool-use,
  permission/approval, final message, changed files into the same `ProviderEvent`
  schema. Not a Codex wrapper — its own adapter + tests. If `claude -p`, mark
  capabilities lower (streaming/approvalEvents/fileEvents/resume only if reliable).
- **Gemini future:** `gemini -p --output-format stream-json`. Must prove stream-json
  schema stability, resume, MCP event visibility, non-hanging permission/failure
  before treated as equivalent. Start as lower-confidence.

## Provider capability model + UI

- `ProviderCapabilities { streaming, resume, approvalEvents, fileEvents,
  skillSelection, mcp, interrupt }`.
- UI shows disabled/lower-confidence providers clearly; disabled providers cannot be
  default.
- **Enforced runner-side, not just in the UI:** the runner rejects a turn/resume to a
  disabled or incapable provider with a typed `UnsupportedProviderRuntimeError`
  (`04-02`) — the UI gate is a convenience, not the boundary.
- The provider-neutral `ask_user` MCP tool works for Claude/Gemini too (it rides the
  MCP proxy); only `approvalEvents`/`interrupt` are provider-capability gated.

## Packaging & distribution (desktop)

- One Electron codebase → Windows + macOS (+ Linux) via `electron-builder`/`forge`.
- **macOS requires a Mac** for code-signing + Apple notarization; build/sign via CI
  (e.g. GitHub Actions with Windows + macOS runners). One push → both signed
  installers.
- **Linux:** ship AppImage/`.deb`; signing/notarization not required (document it as
  unsigned/self-signed) — only Windows + macOS are signed.
- Auto-update wiring (update server + signature verification). Mobile (iOS/Android)
  is out of scope.

## Documentation & migration notes

- Operator docs: provider/account setup, YOLO posture (and that YOLO=true disables
  gating), Codex sandbox requirement for real dangerous-command safety.
- Migration notes: retiring the `="approve"` hack; admin-web orchestration moved to
  the runner; `ExecutePrompt` fallback retirement criteria.

## Acceptance

- Core depends on provider interfaces, not Codex classes; Codex registered as
  implemented; Claude/Gemini as placeholders.
- UI clearly shows unavailable providers.
- Desktop app builds + is signed on Windows and macOS from one codebase via CI.

## Tests

- T-19 fallback `ExecutePrompt` still passes; placeholders visible-but-unavailable.
- packaging smoke: signed installer launches on Win + macOS.

## Definition of Done (checklist)

> Implemented in `apps/local-runner/internal/runner/` (`provider_registry.go`
> Selectable/DefaultProviderKey + `createRun` enforcement) with `phase7_test.go`;
> desktop packaging in `apps/desktop-flowpilot/package.json` (electron-builder) +
> `.github/workflows/desktop-release.yml`; docs in `04-07-Operator-Docs.md` +
> `04-07-Migration-Notes.md`. `go vet` clean; all Phase 7 tests pass; no regressions
> vs the HEAD baseline; desktop `tsc`/`vite build` green.
> **Deferred:** real Claude/Gemini adapters (their own adapters + tests, future);
> signed installers + an actual packaging smoke run (needs CI runners + Apple/Win
> signing secrets — the config + workflow are in, signing activates when secrets are
> present); auto-update server wiring.

- [x] Core depends on provider **interfaces** (`ProviderRuntimeAdapter` + registry), not Codex classes; Codex registered (fake-backed by default; live `codexAdapter` via `ProviderRegistryFor` when `FLOWPILOT_CODEX_APPSERVER` is set).
- [x] Claude/Gemini are **first-class placeholder adapters** (`placeholderAdapter` returns the typed `UnsupportedProviderRuntimeError`, disabled capabilities), visible-but-unavailable (T-19); never the default (`DefaultProviderKey` skips non-available); `Adapter()`/`Selectable()` both gate on availability.
- [x] Provider capability model surfaced (`/admin/providers` returns status + capability flags); **runner-side rejection** of disabled/incapable providers with the typed `UnsupportedProviderRuntimeError` envelope (`createRun` + the `startTurn` adapter guard), not UI-only. _(Desktop capability badges are a thin-client display detail on top of this data.)_
- [x] Packaging targets configured: Linux AppImage/`.deb` (unsigned, documented), Windows nsis, macOS dmg/zip with notarize (electron-builder `build` config). _Producing a **signed** installer + the launch smoke run on Win/macOS is the CI-with-secrets acceptance step._
- [x] Electron build → Windows + macOS + Linux from one codebase via the CI matrix (`.github/workflows/desktop-release.yml`); macOS signed/notarized **when secrets present**; electron-builder GitHub publish target configured for auto-update. _Running the matrix + auto-update server are the deploy/secrets steps._
- [x] Operator docs (YOLO posture incl. YOLO=true disables gating, sandbox requirement) + migration notes (retire `="approve"`, orchestration moved to the runner, `ExecutePrompt` retirement criteria).
- [x] **Review gate:** AI review complete this pass; _human sign-off pending._
