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

## Packaging & distribution (desktop)

- One Electron codebase → Windows + macOS (+ Linux) via `electron-builder`/`forge`.
- **macOS requires a Mac** for code-signing + Apple notarization; build/sign via CI
  (e.g. GitHub Actions with Windows + macOS runners). One push → both signed
  installers.
- Auto-update wiring. Mobile (iOS/Android) is out of scope.

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

- [ ] Core depends on provider interfaces, not Codex classes; Codex registered implemented.
- [ ] Claude/Gemini placeholders visible-but-unavailable (T-19); cannot be default.
- [ ] Provider capability model + UI surfaces disabled/lower-confidence clearly.
- [ ] Electron build → Windows + macOS from one codebase; macOS signed/notarized via CI; auto-update wired.
- [ ] Operator docs (YOLO posture, sandbox requirement) + migration notes (retire `="approve"`, orchestration moved to runner).
- [ ] **Review gate:** human + AI review this checklist after the phase.
