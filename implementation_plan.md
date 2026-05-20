# Implementation Plan: Site-Wide Dark Mode for `apps/admin-web`

## 1. Objective

Implement CP-14 dark mode in `apps/admin-web` as a cross-cutting UI architecture change that delivers:

- persisted `light | dark | system` preference
- no-flash startup theme bootstrap before React paint
- one canonical semantic token source for light and dark palettes
- dark-mode parity across shared shell, common components, and all current routes
- route-level verification so no major page remains light-only

This document is architecture-only by request. It intentionally skips 4C summary and TDD signatures.

## 2. Current State

The existing Vite/TanStack admin app already provides a useful base for dark mode, but it is not yet architected for full site-wide theming.

Observed structure:

- App entry: `apps/admin-web/src/main.tsx`
- App composition: `apps/admin-web/src/app.tsx`
- Root route: `apps/admin-web/src/routes/__root.tsx`
- Authenticated layout shell: `apps/admin-web/src/components/layout/app-shell.tsx`
- Public login route: `apps/admin-web/src/routes/login.tsx`
- Global CSS duplicated across:
  - `apps/admin-web/src/styles.css`
  - `apps/admin-web/src/app/globals.css`
- Zustand is already in use in `apps/admin-web/src/features/auth/use-auth.ts`

Current gaps relevant to CP-14:

- no theme state feature exists yet
- no pre-React bootstrap script exists in `apps/admin-web/index.html`
- token definitions are duplicated across two CSS entrypoints, which will cause drift once dark tokens are added
- shared UI primitives still contain some light-biased values such as hard-coded hover colors
- pages and overlays have not been audited route-by-route for dark contrast parity

## 3. Architectural Decision Summary

### Recommended approach

Implement dark mode with a dedicated client-side `features/theme` slice backed by Zustand, a single root HTML `.dark` contract, and one canonical CSS token source in the Vite app.

This is the recommended approach because it aligns with CP-14 and existing app architecture:

- Zustand is already an accepted lightweight UI state mechanism in this app
- Vite `index.html` can run the no-flash bootstrap script before `main.tsx`
- Tailwind v4 token mapping already exists and can be consolidated instead of replaced
- TanStack Router layout composition provides a single place to mount a theme controller

### Rejected alternatives

#### Option A: React Context as theme source of truth

Not recommended.

- It duplicates the state-management role already assigned to Zustand
- persistence and cross-component reads become more ad hoc
- it gives no advantage over Zustand for this scope

#### Option B: CSS-only system dark mode without application state

Not recommended.

- it cannot satisfy explicit `light | dark | system` preference persistence cleanly
- it cannot expose a reliable three-state UI switcher
- it weakens testability of theme selection behavior

## 4. Target Architecture

### 4.1 Theme feature boundary

Create a dedicated feature under:

`apps/admin-web/src/features/theme/`

Planned files:

- `theme-store.ts`
- `theme-storage.ts`
- `theme-dom.ts`
- `theme-controller.tsx`
- `use-theme.ts`
- `theme-bootstrap.ts`

Responsibilities:

- `theme-store.ts`
  - owns Zustand theme state
  - stores `preference`
  - derives `resolvedTheme`
  - exposes `setPreference`
  - exposes `onSystemPreferenceChange`

- `theme-storage.ts`
  - owns the canonical storage key: `"flowpilot-theme-preference"`
  - isolates safe localStorage read/write behavior
  - normalizes unknown stored values to `system`

- `theme-dom.ts`
  - centralizes all DOM writes to `document.documentElement`
  - applies/removes the `.dark` class
  - optionally sets `color-scheme` for native control rendering parity

- `theme-controller.tsx`
  - mounts once inside the React app
  - subscribes to system media query changes
  - syncs store state with DOM state
  - prevents theme logic from leaking into arbitrary route components

- `use-theme.ts`
  - exposes the narrow public hook API used by UI components

- `theme-bootstrap.ts`
  - provides shared bootstrap-safe helpers for reading preference and resolving theme
  - keeps bootstrap logic aligned with runtime logic

### 4.2 State model

Canonical types:

```ts
export type ThemePreference = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";
```

Recommended store shape:

```ts
interface ThemeState {
  preference: ThemePreference;
  resolvedTheme: ResolvedTheme;
  setPreference: (value: ThemePreference) => void;
  onSystemPreferenceChange: () => void;
}
```

Behavior rules:

- initial preference loads from `"flowpilot-theme-preference"`
- invalid or missing persisted value resolves to `system`
- `resolvedTheme` depends on `window.matchMedia("(prefers-color-scheme: dark)")` when preference is `system`
- explicit `light` or `dark` bypass system preference
- every state change must update both storage and root DOM theme

### 4.3 Root theme application contract

Theme application standard:

- source of truth for visual mode is the `.dark` class on `<html>`
- bootstrap script runs in `apps/admin-web/index.html` before `src/main.tsx`
- React runtime does not guess the initial theme; it hydrates around the already-applied HTML class

Flow:

1. `index.html` inline bootstrap script reads `"flowpilot-theme-preference"`
2. script resolves final theme with system preference fallback
3. script toggles `document.documentElement.classList`
4. React mounts
5. `theme-controller.tsx` attaches media-query listener and keeps runtime state synchronized

This split is important because the bootstrap path solves FOWT while the React controller solves ongoing updates.

### 4.4 CSS token architecture

For the Vite route tree, `apps/admin-web/src/styles.css` should become the single canonical token source.

Reasoning:

- `src/main.tsx` imports `./styles.css`
- the Vite route tree appears to be the active implementation surface for CP-14
- `src/app/globals.css` belongs to the parallel Next-style app tree and should not become a second source of truth for the same UI contract

Required token groups:

- `--background`
- `--foreground`
- `--muted`
- `--muted-foreground`
- `--card`
- `--card-foreground`
- `--border`
- `--accent`
- `--accent-foreground`
- `--warning`
- `--danger`
- `--success`

Architecture rules:

- light tokens live under `:root`
- dark tokens live under `.dark`
- `@theme inline` remains semantic and maps only to CSS variables
- component code uses semantic Tailwind utilities instead of raw grayscale values
- decorative gradients and shadows must also become token-driven or dual-mode aware

### 4.5 UI composition points

Mount points:

- `apps/admin-web/index.html`
  - bootstrap script

- `apps/admin-web/src/app.tsx`
  - mount `ThemeController` near other app-wide providers

- `apps/admin-web/src/routes/__root.tsx`
  - optional alternative mount point if route-root lifecycle is preferred

- `apps/admin-web/src/components/layout/app-shell.tsx`
  - global theme switcher placement for authenticated users

- `apps/admin-web/src/routes/login.tsx`
  - public route dark-mode parity and optional public switcher placement if desired

Recommended composition:

- put the bootstrap in `index.html`
- put `ThemeController` in `src/app.tsx`
- put the visible switcher in `AppShell`
- keep route pages free of direct DOM theme mutations

## 5. Shared UI Audit Strategy

Dark mode should be implemented from shared primitives outward. This minimizes repeated route work.

### Tier 1: Foundation surfaces

Audit first:

- `src/styles.css`
- `src/components/layout/app-shell.tsx`
- `src/components/ui/button.tsx`
- `src/components/common/page-frame.tsx`
- `src/components/common/placeholder-page.tsx`
- `src/routes/login.tsx`

Expected changes:

- replace remaining hard-coded light-biased values with semantic classes
- adjust panel backgrounds, borders, shadows, and noise backgrounds for dark readability
- ensure inputs, buttons, and helper text have dark contrast parity

### Tier 2: Shared route patterns

Audit route clusters that likely reuse similar card, form, and panel structures:

- dashboard
- projects list/detail subroutes
- settings routes
- MCP server routes
- workflow and output routes
- teams and AI runs routes

Expected changes:

- fix isolated hard-coded `bg-white`, `text-black`, `border-gray-*`, `shadow-black/*`
- verify empty states, tables, badges, and status messaging in dark mode
- verify translucent layers and blur surfaces remain readable

### Tier 3: Overlays and edge states

Pay special attention to:

- overlays and backdrops
- disabled states
- focus states
- validation and error states
- text selection
- hover and pressed states

Known early risk:

- `src/routes/_authenticated/settings/mcp-servers.tsx` already uses an explicit `dark:` overlay treatment, which suggests other pages may contain one-off color handling that must be normalized rather than expanded.

## 6. Theme Switcher Architecture

Provide one reusable presentation component, for example:

- `src/features/theme/components/theme-switcher.tsx`

Responsibilities:

- render the three choices: `Light`, `Dark`, `System`
- read current `preference` and `resolvedTheme`
- update immediately through `setPreference`

Placement strategy:

- primary location: authenticated app shell header or top utility row
- secondary optional location: settings page

Architecture rules:

- the switcher reads state through `useTheme()`
- the switcher does not manipulate DOM classes directly
- visual active state should reflect `preference`, not just `resolvedTheme`

## 7. Testing and Verification Architecture

### Automated coverage

Add or update tests in these categories:

- store tests
  - preference persistence
  - invalid-storage fallback
  - `system` resolution behavior

- DOM/theme sync tests
  - `.dark` class application
  - system preference change listener behavior

- component tests
  - switcher updates selection state
  - app shell renders toggle safely

- route smoke tests
  - key route components render under dark-mode class without regressions

### Manual visual QA matrix

Check each route in:

- light preference
- dark preference
- system preference with OS light
- system preference with OS dark

Validate:

- no flash of wrong theme on refresh
- body, shell, cards, panels, dialogs, forms, tables, and placeholders all adapt
- contrast is acceptable for primary text, secondary text, borders, buttons, and alerts

## 8. Rollout Sequence

1. Build theme foundation in `src/features/theme`
2. Add no-flash bootstrap in `index.html`
3. Mount `ThemeController` in `src/app.tsx`
4. Consolidate Vite token architecture into `src/styles.css`
5. Refactor shared shell and common primitives
6. Add visible theme switcher in app shell
7. Audit public login route
8. Audit authenticated route families
9. Add/expand smoke tests and manual QA checklist

This sequence is intentionally foundation-first so later route work becomes mostly cleanup instead of parallel theming implementations.

## 9. Risks and Mitigations

### Risk 1: Dual CSS entrypoint drift

Cause:

- `src/styles.css` and `src/app/globals.css` currently duplicate token definitions

Mitigation:

- treat `src/styles.css` as canonical for the Vite admin app
- do not maintain parallel dark token definitions for the same runtime surface

### Risk 2: Bootstrap/runtime mismatch

Cause:

- inline bootstrap and React runtime may resolve theme differently

Mitigation:

- share normalization and resolution helpers via `theme-bootstrap.ts`
- keep one storage key and one resolution algorithm

### Risk 3: Hard-coded component colors survive audit

Cause:

- current primitives and route screens still contain direct color literals

Mitigation:

- audit shared components before route pages
- search for raw color utilities and token violations as part of implementation

### Risk 4: System preference listener leaks

Cause:

- `matchMedia` listeners can accumulate if mounted in multiple places

Mitigation:

- mount exactly one `ThemeController`
- centralize listener registration and cleanup there

## 10. Definition of Done

CP-14 is architecturally complete when:

- `apps/admin-web` has a dedicated theme feature with Zustand-based preference state
- the canonical storage key is `"flowpilot-theme-preference"`
- `index.html` applies the correct `.dark` class before React paint
- theme DOM mutation is centralized rather than scattered across components
- Vite global tokens are consolidated into one canonical source
- shared shell and common primitives are dark-safe
- all current public and authenticated routes have been audited for dark-mode parity
- route smoke coverage and manual visual QA checklist are in place
