# CP-14: Site-Wide Dark Mode

**Maps from:** SD-02 (Architecture), CP-01 (Foundation Setup)
**Phase:** 10 (cross-cutting UX hardening after core admin-web screens exist)
**Depends on:** CP-01, CP-04, CP-06, CP-07, CP-08

---

## 1. Goal

Implement a complete dark mode for the entire `admin-web` site, not just a theme toggle.

This phase must deliver:

- a persistent light/dark/system theme model
- a no-flash theme bootstrap at app startup
- semantic design tokens for both light and dark palettes
- consistent dark styling across all shared layouts, screens, states, and overlays
- a route-by-route audit so no major page remains light-only

---

## 2. Why This Is A Separate CP

Dark mode touches almost every frontend surface:

- root app bootstrapping
- CSS token architecture
- shared layout primitives
- shadcn/ui wrappers
- route-level screens
- visual QA and accessibility contrast

It should be treated as a dedicated cross-cutting phase instead of being mixed into unrelated feature CPs.

SD-02 already reserves Zustand for lightweight UI state such as dark mode, so CP-14 should implement that decision rather than introducing a second theme state mechanism.

---

## 3. Scope

### In scope

- `apps/admin-web` only
- theme preference: `light`, `dark`, `system`
- theme persistence in browser storage
- root HTML/class bootstrapping before React paint
- dark tokens for background, foreground, card, border, muted, accent, success, warning, danger
- layout surfaces: app shell, navigation, panels, dialogs, tables, forms, empty states, loading states
- all authenticated and public routes currently present in `src/routes/`
- route smoke tests and visual QA checklist

### Out of scope

- user-profile persistence of theme preference in Supabase
- per-project theme branding
- redesigning the product visual language beyond what is needed for dark mode parity
- dark-mode support for non-admin surfaces outside `apps/admin-web`

---

## 4. Architecture Decisions

### 4.1 Theme State Source Of Truth

Use Zustand for theme preference because SD-02 already assigns lightweight UI state to Zustand.

The store and the bootstrap script MUST use the canonical localStorage key: `"flowpilot-theme-preference"`.

Suggested store shape:

```typescript
// src/features/theme/theme-store.ts
export type ThemePreference = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";

export interface ThemeState {
  preference: ThemePreference;
  resolvedTheme: ResolvedTheme;
  setPreference: (value: ThemePreference) => void;
  // Triggered internally or by event listener on system media query changes
  onSystemPreferenceChange: () => void;
}
```

The store should:

- persist `preference` using the `"flowpilot-theme-preference"` storage key.
- derive `resolvedTheme` from `window.matchMedia("(prefers-color-scheme: dark)").matches` when preference is `system`.
- Note: `window.matchMedia` is assumed to execute in a Client-Side Rendering (CSR) environment.
- expose the `useTheme()` custom React hook:
  ```typescript
  // src/features/theme/use-theme.ts or theme-store.ts export
  export function useTheme(): {
    preference: ThemePreference;
    resolvedTheme: ResolvedTheme;
    setPreference: (value: ThemePreference) => void;
  }
  ```

### 4.2 Root Theme Application

Standardize on a `.dark` class applied to the root HTML element.

Requirements:

- the root class must be applied before the React tree paints to prevent flash-of-wrong-theme (FOWT)
- the current theme must survive page refresh by loading `"flowpilot-theme-preference"` in a tiny script in `index.html`
- system preference changes should update the resolved theme when preference is `system`

Recommended implementation:

1. add a tiny bootstrap script in `index.html` that reads `"flowpilot-theme-preference"` and applies the `.dark` class to `document.documentElement`
2. add a React-side theme controller/listener that listens to media query changes (`window.matchMedia("(prefers-color-scheme: dark)").addEventListener(...)`) and calls `onSystemPreferenceChange` to keep the store and classList in sync
3. centralize all theme DOM writes through one helper instead of mutating `classList` from multiple components

### 4.3 Token Strategy

The current app already defines custom CSS variables in:

- `apps/admin-web/src/styles.css`
- `apps/admin-web/src/app/globals.css`

CP-14 should consolidate this into one canonical token source so light and dark palettes do not drift.

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

Add dark equivalents under `.dark` and keep component styling semantic:

- use `bg-background`, `text-foreground`, `bg-card`, `border-border`
- avoid hard-coded `bg-white`, `text-black`, `border-gray-*`, `shadow-black/..` in feature screens

### 4.4 Tailwind / shadcn Contract

Tailwind utility usage should rely on semantic tokens first, not one-off light-only colors.

CP-14 must:

- support the `dark:` variant where it adds clarity
- prefer token-driven classes so most components adapt automatically
- patch shared shadcn wrappers if they leak light-only colors
- ensure overlays, backdrops, focus rings, and selection colors still read correctly in dark mode

### 4.5 App Entry Integration

Expected touch points:

- `apps/admin-web/src/main.tsx`
- `apps/admin-web/src/app.tsx`
- `apps/admin-web/src/routes/__root.tsx`
- `apps/admin-web/src/components/layout/app-shell.tsx`
- `apps/admin-web/index.html`

The root app should expose one visible theme switcher in a globally reachable location such as the app shell header or settings navigation.

---

## 5. Implementation Plan

### 5.1 Theme Foundation

Create a small dedicated theme feature:

- `src/features/theme/theme-store.ts`
- `src/features/theme/theme-provider.tsx` or `theme-controller.tsx`
- `src/features/theme/theme-storage.ts`
- `src/features/theme/theme-bootstrap.ts`

Responsibilities:

- read/write local storage
- resolve system theme
- subscribe to system preference changes
- apply root `.dark` class
- expose `useTheme()` hook

### 5.2 CSS Refactor

Refactor global CSS so theme tokens live in one place.

Tasks:

- remove duplicate token definitions across `src/styles.css` and `src/app/globals.css`
- define light palette under `:root`
- define dark palette under `.dark`
- update `body`, `::selection`, noise backgrounds, and panel shadows for both modes
- ensure decorative backgrounds remain subtle and readable in dark mode

### 5.3 Global UI Controls

Add a theme switcher with three options:

- `Light`
- `Dark`
- `System`

Recommended surfaces:

- app shell header quick toggle
- settings page control for explicit preference selection

The UI must clearly show the active selection and update immediately without reload.

### 5.4 Shared Component Audit

Audit shared components first because route coverage improves automatically once primitives are fixed.

Priority areas:

- `src/components/layout/*`
- `src/components/common/*`
- `src/components/ui/*`

Things to fix:

- hard-coded light backgrounds
- hard-coded dark text on muted surfaces
- borders and dividers that disappear in dark mode
- shadows that become muddy on dark surfaces
- translucent overlays that are too bright at night

### 5.5 Route Audit

Every current route should be verified in both themes:

- `/login`
- `/dashboard`
- `/teams`
- `/ai-runs`
- `/projects`
- `/projects/create`
- `/projects/$projectId/business-logic`
- `/projects/$projectId/tech-specs`
- `/projects/$projectId/coding-plan`
- `/projects/$projectId/master-schedule`
- `/projects/$projectId/tasks`
- `/projects/$projectId/members`
- `/projects/$projectId/workflows`
- `/projects/$projectId/settings`
- `/settings/integrations`
- `/settings/mcp-servers`
- `/settings/mcp-servers/create`
- `/settings/mcp-servers/mcp-connect-test`
- `/settings/prompt-templates`
- `/settings/runner`

Audit checklist per route:

- page background
- cards and panels
- inputs and selects
- tables and list rows
- status badges
- empty states
- modal/sheet/dialog overlays
- hover and focus states
- loading and error states

### 5.6 Content Surfaces

Special attention is required for rich-content surfaces because they often break first in dark mode:

- Markdown viewers
- prompt editors
- code/pre blocks
- artifact previews
- test consoles
- charts or analytics cards if added by earlier CPs
- **Workflow step status badges (from CP-07)**: These require state-driven semantic background/text/pulse classes that must preserve a high contrast ratio in dark mode.

These surfaces must not rely on browser defaults alone.

---

## 6. Suggested File Targets

Likely implementation files include:

```text
apps/admin-web/index.html
apps/admin-web/src/main.tsx
apps/admin-web/src/app.tsx
apps/admin-web/src/styles.css
apps/admin-web/src/app/globals.css
apps/admin-web/src/routes/__root.tsx
apps/admin-web/src/components/layout/app-shell.tsx
apps/admin-web/src/features/theme/theme-store.ts
apps/admin-web/src/features/theme/theme-provider.tsx
apps/admin-web/src/features/theme/theme-storage.ts
apps/admin-web/src/features/theme/theme-bootstrap.ts
```

Additional route and component files should be updated only where audit findings require them.

---

## 7. Testing Plan

### 7.1 Unit / Component Tests

- theme store persists `light`, `dark`, `system`
- resolved theme follows system preference in `system` mode
- root `.dark` class is applied and removed correctly
- theme toggle UI reflects active state

### 7.2 Route Smoke Tests

At minimum, add smoke coverage for:

- app boot in saved dark mode
- switching theme from the app shell
- login page in dark mode
- one project detail route in dark mode
- one settings route in dark mode

### 7.3 Manual QA

Manual verification checklist:

- refresh does not flash the wrong theme
- keyboard focus is visible in both modes
- contrast is acceptable for body text, muted text, and status badges
- translucent overlays do not wash out content
- charts, markdown, and code blocks remain readable

---

## 8. Definition of Done

- [ ] A new theme module exists in `src/features/theme/` with Zustand-backed preference state
- [ ] Theme preference supports `light`, `dark`, and `system`
- [ ] Theme preference persists across browser refresh
- [ ] Both `index.html` bootstrap script and Zustand store use the identical localStorage key `"flowpilot-theme-preference"` to ensure seamless boot state
- [ ] Root HTML theme class is applied before React paint to prevent flash
- [ ] Global CSS tokens define both light and dark palettes
- [ ] Token definitions are consolidated to one canonical source (`styles.css` and `globals.css` refactored)
- [ ] App shell exposes a working theme switcher
- [ ] Shared UI primitives no longer depend on light-only hard-coded colors
- [ ] All current public and authenticated routes are verified in both themes
- [ ] Login, dashboard, project detail, workflow, and settings surfaces render correctly in dark mode
- [ ] Dialogs, sheets, tables, badges, inputs, markdown, and code blocks remain readable in dark mode
- [ ] Theme behavior is covered by automated tests for store/bootstrap behavior
- [ ] No major contrast regressions remain in manual QA

---

## 9. Notes

- CP-14 should land after the main route surfaces exist, otherwise the team will pay for the same audit multiple times.
- Prefer semantic token migration over sprinkling `dark:` utilities everywhere.
- If some pages still use hard-coded light palette values, fix shared primitives first, then patch route-level exceptions.
