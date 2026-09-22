# Task-405: Desktop UI Consistency Revamp (Design Tokens + Component Audit)

## Metadata

- Document ID: `Task-405`
- Title: `Desktop UI Consistency Revamp — Token Scales, Motion, Icon Unification`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: `None (standalone UX hardening task)`
- Child Documents: `None`
- Related Documents: [Task-404: Desktop Attention Queue](./Task-404-Desktop-Attention-Queue.md)
- Replaces: `None`
- Tags: `desktop, ux, design-tokens, css, revamp`
- Feature Keys: `desktop-ui-consistency`

## AI Quick View

### Summary

- `desktop-flowpilot` feels less consistent/smooth than Devin Desktop. Root cause found by inspecting Devin's bundle: Devin inherits a single `--vscode-*` token source (one color/font/icon system, 4px sash rhythm, codicons), while our `styles.css` (~6.9k lines) has a good `:root` color/font block but **no spacing, elevation, radius, or motion scales** — every component freelances.
- This task hardens the token layer and aligns components to it: spacing scale, surface elevation, radius scale, motion tokens, and one icon strategy — a visual-only refactor with zero behavior change.

### Current Ask

- Introduce the missing token scales in `styles.css`, migrate components off ad-hoc px values, unify icon usage, and add lightweight guardrails (token-lint or a documented scale) so the consistency is enforceable going forward.

### Key Decisions

- `T-1` Add token scales: `--space-1..6` (4/8/12/16/24/32), `--radius-{sm,md,lg}`, `--elev-{1,2,3}` (bg + single shadow spec), `--dur-{fast,med}` + `--ease-standard`, `--font-size-{xs..xl}`, `--line-{tight,normal}`.
- `T-2` Visual-only scope: same DOM structure, same events, same layout intents — only values migrate to tokens. No component API or behavior changes.
- `T-3` Icon unification: pick one icon strategy already in the repo (codicon-style glyph set or current inline SVGs — audit decides, one wins); ban raw emoji/text glyphs in chrome.
- `T-4` Guardrail: a `STYLE-TOKENS.md` section or a tiny stylelint/grep rule set that fails on hardcoded `px` spacing/radius outside the token block (scoped to `styles.css` chrome, not third-party).

### Constraints

- Pure presentation change: no edits to `state/`, `client/`, event handling, or any Go code.
- Dark theme first (current `--bg #0d0d0d` family preserved); keep brand tokens (`--claude-brand` etc.) untouched.
- Additive tests only; visual verification via screenshots; any pre-existing test failure → stop and report (safe-fix-contract).
- Large mechanical diff expected — split into per-component-area commits to stay reviewable.

### Open Questions

- `Q-1` Resolved — tokens + existing per-component classes; no utility framework.
- `Q-2` Resolved — keep `Inter` + `ui-monospace` (system stack fallback already in place).

### Source Refs

- `apps/desktop-flowpilot/src/styles.css` (`:root` block lines 1–35, component rules throughout), `src/components/*` (chrome: `Navigator`, `ChatWorkspace`, `ChatInput`, `FlowStepTimeline`, `FlowTimelineSidebar`, `ChatPosturePanel`, card family).
- Reference model: Devin Desktop bundle (`out/vs/sessions/sessions.desktop.main.css`) — single `--vscode-*` token source, `--vscode-chat-font-family`, monaco/system mono pairing, codicons, 4px sash rhythm.

## 1. Goal

Make the desktop app look and feel as consistent and smooth as Devin Desktop by centralizing all visual values into a token system and aligning every chrome component to it — with zero functional change.

## 2. Parent Links

- coding plan: `None`
- tech design: `None`
- system spec: `None`
- specific upstream ids: related to `Task-404` (attention queue should land on the new tokens).

## 3. Trigger

Multi-run vibe usage raises time-in-app; visual inconsistency (mixed paddings, ad-hoc radii, no shared motion) is now a product problem, not polish debt.

## 4. Exact Change

- `T-1` Extend `:root` in `styles.css` with spacing/radius/elevation/motion/typography scales (names above); keep existing color tokens.
- `T-2` Migrate component rules: replace hardcoded `px` padding/margin/gap/radius/shadow/transition values with tokens, area by area (Navigator → chat column → cards → panels → modals).
- `T-3` Unify transitions: all hover/focus/state changes use `--dur-fast var(--ease-standard)`; layout shifts use `--dur-med`; remove one-off durations.
- `T-4` Icon audit + unification (single source); replace stray text/emoji glyphs in chrome.
- `T-5` Guardrail doc/rule + a `styles.tokens.test.ts` (or grep check) asserting token usage coverage on migrated areas.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/styles.css` primarily; component `.tsx` files only where inline styles exist; new token test; `docs/` or `FEATURE-KEYS.md` note.
- modules: desktop presentation layer only.
- routes: none.
- tables: none.

## 6. Code Guide Signatures

```css
/* apps/desktop-flowpilot/src/styles.css :root additions — T-1 */
--space-1: 4px;  --space-2: 8px;  --space-3: 12px;
--space-4: 16px; --space-5: 24px; --space-6: 32px;
--radius-sm: 4px; --radius-md: 8px; --radius-lg: 12px;
--elev-1: /* raised surface bg + single shadow spec */;
--elev-2: /* overlay */;
--elev-3: /* modal */;
--dur-fast: 120ms; --dur-med: 200ms;
--ease-standard: cubic-bezier(0.2, 0, 0, 1);
--font-size-xs: 11px; --font-size-sm: 12px; --font-size-md: 13px;
--font-size-lg: 15px; --font-size-xl: 18px;
--line-tight: 1.25; --line-normal: 1.45;
```

```ts
// apps/desktop-flowpilot/src/styles.tokens.test.ts — guardrail (T-5)
// Grep-based assertion over styles.css: migrated sections contain no
// hardcoded `px` padding/margin/gap/radius/transition-duration outside :root.
test("migrated sections use tokens for spacing/radius/motion", () => { /* … */ })
```

Component `.tsx` files: **unchanged signatures** — migration touches class rules and inline `style={{}}` values only (T-2).

## 7. Test Signatures

- `test("root defines spacing scale 4-32")` — all `--space-*` tokens present.
- `test("root defines radius/elevation/motion scales")` — `--radius-*`, `--elev-*`, `--dur-*`, `--ease-standard` present.
- `test("migrated sections use tokens for spacing/radius/motion")` — regex scan: no `padding|margin|gap: <n>px` / `border-radius: <n>px` / `transition .* ms` literals in migrated blocks.
- `test("icon imports come from a single source")` — no ad-hoc emoji/text glyph usage in migrated chrome components.

## 8. Acceptance Check

- Token scales exist and migrated areas contain no hardcoded spacing/radius/duration outside the `:root` block (lint/grep proof in test).
- Visual parity screenshots (before/after) for Navigator, chat column, one card type, posture panel — no unintended layout shifts.
- Full existing desktop vitest suite green, untouched.

## 9. Out of Scope

- Light theme; redesigning information architecture or component hierarchy; Tailwind/utility-framework adoption; TUI styling; new features.

## 10. Definition of Done

- [ ] `:root` carries spacing/radius/elevation/motion/typography scales exactly as §6.
- [ ] All §7 tests exist, green, additive-only; migrated-area lint shows zero hardcoded visual values.
- [ ] Icon strategy unified (one source, no stray glyphs).
- [ ] Before/after screenshots show equal-or-better layout, smoother consistent motion.
- [ ] Pre-existing suite untouched and green; old failure → STOP + report.
- [ ] Provider parity: N/A — pure CSS/TS presentation, no provider path (evidence noted in CA).
- [ ] CA ledger entry under `feature_key: desktop-ui-consistency`; `detect_changes` shows only expected files.

## 11. Completion Notes

- result: `TBD`
- follow-ups: `TBD`
- upstream docs updated: `TBD`
