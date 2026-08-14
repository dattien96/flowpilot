---
id: CA-483
feature_key: cli-tui
title: Grok image attach via .tmp/images path fallback
date: 2026-08-14
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-482-tui-altv-image-paste, CA-443 clipboard images
will_not_undo: CA-482 Alt+V routing; Codex/Claude native multimodal; Grok Capabilities.Vision stays false (ACP image blocks still unsupported)
```

## Intent

Operator: Grok cannot use ACP multimodal image blocks (`promptCapabilities.image=false`).
Fallback: TUI Alt+V / Desktop paste → turn `attachments[]` → runner writes files under
`<cwd>/.tmp/images/<turn>/` → absolute paths appended to the text prompt so Grok can
open them with file tools.

## Change

### Runner (`internal/runner`)

- `grok_attachments.go`: `writeGrokImagePathFallback`, `appendGrokImagePathsToPrompt`,
  `sweepGrokImagePathFallback`
- `grokAdapter.SendTurn`: write attachments before `session/prompt`, inject path block,
  defer cleanup after turn (same lifecycle as Codex temp images)
- Boot: sweep no-cwd temp root in `newInteractiveService`
- `Capabilities.Vision` remains **false** (honest ACP signal; old capability test green)

### UI gates

- TUI `SupportsImages`: add `grok` (path fallback)
- Desktop `VISION_PROVIDERS`: add `grok`
- Tips/README: codex/claude native + grok path-fallback

### Repo

- `.gitignore`: `.tmp/`

## Provider classification

| Provider | Path |
|----------|------|
| Codex | Unchanged native `localImage` paths under flowpilot-attach |
| Claude | Unchanged base64 image content blocks |
| Grok | **New** path-fallback only (this CA) |

## Tests (additive only)

- `grok_attachments_test.go` — write/cleanup/sweep/prompt suffix
- `grok_image_path_fallback_sendturn_test.go` — fake ACP asserts path in session/prompt
- `image_grok_support_test.go` — SupportsImages includes grok
- Old `TestGrokAdapterCapabilitiesMatchProvenSet` **untouched, green** (Vision still false)

## Residual

- Grok does not “see” pixels natively; quality depends on the model opening files via tools.
- Per-turn dir cleaned when SendTurn returns; re-attach for a later turn.
- Project-local `.tmp/images` under active cwd; boot sweep only covers no-cwd temp root.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: Grok image attach via cwd/.tmp/images path fallback injected into text prompt
# --->8---
