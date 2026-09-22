# CA-919: Desktop Devin-Dark Restyle

## Summary

Restyled `apps/desktop-flowpilot` to follow the Devin desktop app's visual
language (extracted from the installed `Devin.app` bundle —
`workbench.desktop.main.css`, `theme-windsurf/themes/windsurf-dark.json`, and
the shipped `InterVariable.woff2`). All changes are token-level in
`styles.css`, so every component picks the new system up through existing
`var(--*)` references — no component code touched.

## Token mapping (old Codex-dark → Devin dark)

| Token | Before | After (Devin) |
|---|---|---|
| `--bg` page | `#0d0d0d` | `#141414` |
| `--bg-2` wash/sidebar | `#161616` | `#191919` |
| `--bg-3` elevated | `#1e1e1e` | `#1f1f1f` |
| `--border` | `#2a2a2a` | `rgba(255,255,255,.05)` hairline |
| `--text` | `#ececec` | `rgba(255,255,255,.9)` |
| `--text-dim` | `#9b9b9b` | `rgba(255,255,255,.52)` |
| `--text-2` | `.55` alpha | `.40` alpha |
| `--prompt-bg`/border | `#2a2a2a`/`#3a3a3a` | white-alpha `.05`/`.08` |
| `--accent` | `#4c8dff` | `#49b0ff` (Devin text accent) |
| `--accent-2` | `#2f6bdb` | `#317cff` (Devin button blue) |
| `--warn` / `--err` / `--ok` / `--ask` | `#f0b429`/`#f85149`/`#3fb950`/`#b07cff` | `#f58e3a`/`#f53b3a`/`#00ec7e`/`#956cde` |
| `--radius-md` / `--radius-lg` | 8px / 12px | 6px / 10px (VS Code cornerRadius medium/large) |
| `--chat-prompt-max` | 630px | 760px (Devin composer width) |
| base font-size | 12px | 13px |

Also:

- **Inter bundled**: copied `InterVariable.woff2` (SIL OFL-licensed font, same
  file Devin ships) into `src/assets/fonts/` + `@font-face` (100–900 range,
  `font-display: swap`) — UI now renders real Inter instead of silently
  falling back to system fonts when Inter isn't installed.
- **Scrollbars**: thin VS Code-style alpha thumbs (`rgba(255,255,255,.14)`
  resting / `.24` hover / `.32` active, transparent track) — replaces default
  OS scrollbars.
- **`::selection`**: accent-tinted `rgba(73,176,255,.3)`.
- **New tokens**: `--surface` (`#1f1f1f`), `--surface-2` (white-alpha `.08`
  hover tint) — previously referenced with per-site fallbacks, now defined
  once.
- **Literal sweep**: ~110 hardcoded rgba/hex literals retargeted to the new
  palette (`76,141,255` → `73,176,255`; old ok/warn/err/ask rgb triplets →
  Devin values); stray `var(--bg-1, …)` typos fixed to `--bg-3`; one raw
  `#2a2a2a` border → `var(--border)`.

## Tests

- `styles.tokens.test.ts` 4/4 pass (token guardrail contract preserved — all
  new values live in `:root` or exempt categories).
- `npx vite build` green; `InterVariable-*.woff2` emitted to `dist/assets`.
- `tsc --noEmit` — only the known pre-existing
  `store.chat-mode-persist.test.ts` error at HEAD.

## Notes

- Scope is dark-theme only; FlowPilot ships dark-only today.
- Provider brand colors (`--claude-brand`, `--gemini-brand`, etc.) kept — they
  are identity colors, not theme palette.
- `--role-main` and `--devin-brand` retargeted to `#49b0ff` for consistency.
- Font file is upstream Inter (OFL), same as Devin's bundle — redistributable.
