# CA-088 Desktop ChatInput Provider Card Picker

## Scope

Replaced the `<select>` dropdown for AI provider selection in the desktop chat controller
(`apps/desktop-flowpilot/src/components/ChatInput.tsx`) with three clickable card buttons —
one each for Codex, Claude, and Gemini.

**Layers touched:**
- `apps/desktop-flowpilot/src/components/ChatInput.tsx` — removed `PROVIDER_OPTIONS`, added
  `CodexIcon` / `ClaudeIcon` / `GeminiIcon` SVG components, added `PROVIDER_CARDS` constant,
  replaced provider `<label><select>` with `<div class="provider-picker">` containing 3 buttons.
- `apps/desktop-flowpilot/src/styles.css` — added brand color CSS variables (`--codex-brand`,
  `--claude-brand`, `--gemini-brand`), changed `.chat-controller-grid` from 3-column to 2-column
  (model + reasoning), added `.provider-picker`, `.provider-picker-cards`, `.provider-card`,
  brand-specific icon colour and selected-state rules, updated responsive breakpoint.

## Completed

- Provider picker renders 3 equal-width cards (Codex / Claude / Gemini) spanning the full width
  of the controller grid row, with brand icon + label in each card.
- Brand icons: OpenAI blossom SVG (Codex), Anthropic "A" lettermark (Claude), Google Gemini
  4-pointed star (Gemini).
- Brand accent colours used for icon tint and selected-state border/background:
  Codex `#e0e0e0`, Claude `#e17554`, Gemini `#7c6ff0`.
- Clicking an unselected card calls `selectProvider(value)`; clicking the already-selected card
  calls `selectProvider(undefined)` — preserving the existing "Auto" (no provider) state.
- `aria-pressed` tracks selected state for accessibility; `disabled` prop propagates from
  `blocked` run state.
- On narrow screens the 3 cards collapse to a single column via the existing media breakpoint.
- No store, contract, or runner changes required — the `selectProvider` action signature is
  unchanged.

## Verification

- `npx tsc --noEmit` on `apps/desktop-flowpilot` — clean, no errors.
- Manual visual verification not performed (no preview server available at time of change);
  layout correctness derivable from CSS grid rules.

## Residual Notes

- SVG paths are inline approximations of official brand marks; if pixel-perfect brand accuracy
  is required, replace with assets sourced directly from each provider's brand kit.
- "Auto" mode (no provider selected) is now implicit (no card highlighted) rather than an
  explicit "Auto" option — this is a UX change worth confirming with the team.
- The `.chat-controller-grid` column count changed from 3 to 2; if a third non-provider field
  is added in the future, the grid template must be updated.
