# Desktop Style Tokens (Task-405)

`src/styles.css` `:root` owns the single token source for chrome visuals —
the Devin/VS Code model: components consume tokens, never ad-hoc values.

## Scales

| Group | Tokens | Values |
|---|---|---|
| Spacing | `--space-1..6` | 4 / 8 / 12 / 16 / 24 / 32 px (4px rhythm) |
| Radius | `--radius-sm/md/lg/pill` | 4 / 8 / 12 / 999 px |
| Elevation | `--elev-1/2/3` | raised / overlay / modal shadow specs |
| Motion | `--dur-fast`, `--dur-med`, `--ease-standard` | 120ms / 200ms / `cubic-bezier(0.2,0,0,1)` |
| Type | `--font-size-xs..xl`, `--line-tight`, `--line-normal` | 11–18px, 1.25/1.45 |

## Rules

- New chrome rules must use `var(--space-*)` for padding/margin/gap,
  `var(--radius-*)` for `border-radius`, and `var(--dur-*) var(--ease-standard)`
  for transitions. Hairline values (< 3px) may stay literal.
- Icons: inline SVG glyphs only (single source — see `SyncGlyph`,
  posture gear icon). No emoji/text glyphs in chrome.
- Migrated areas are enforced by `src/styles.tokens.test.ts` — it scans the
  selector list in `MIGRATED_PREFIXES` and fails on hardcoded px/ms.
  To migrate a new area: convert its rules to tokens, then add its selector
  prefix to that list.
