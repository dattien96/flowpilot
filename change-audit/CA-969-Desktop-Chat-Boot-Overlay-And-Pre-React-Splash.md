# CA-969 — Desktop chat boot overlay + pre-React splash (silent startup loading)

## Summary

Desktop app startup had three silent gaps: (1) an empty dark window while the
renderer bundle loaded (`index.html` had no pre-React content); (2) the
"Bootstrapping desktop workspace…" card only covered the auth/runner probe;
(3) once `phase === "authenticated"`, `ChatWorkspace` rendered instantly while
`loadProjects()` was still running its sequential pipeline (projects → catalog
→ skills → accounts → posture) — the composer stayed dead (`canSend` needs
`localProviders` + `providerAccounts`) with zero loading indication. Users saw
a ready-looking chat that could not send for seconds.

## What changed

- `apps/desktop-flowpilot/index.html` — inline pre-React splash (spinner +
  wordmark, pure CSS, CSP-safe) inside `#root`; replaced by first render.
- `apps/desktop-flowpilot/src/state/store.ts` — new `chatBoot` state
  (`{status: idle|loading|ready|failed, step, error}`) driven inside
  `loadProjects()`. Steps: `projects → catalog → accounts → session`.
  Boot-critical failures (projects/catalog/accounts — the data `canSend` needs)
  flip status to `failed` with the first error; skills/posture/mode-restore
  failures stay non-fatal. Once `ready`, later `loadProjects` calls refresh
  silently — the gate never re-locks over a usable workspace. A trailing
  `.catch` guarantees a stray throw can never wedge the overlay on "loading".
- `apps/desktop-flowpilot/src/state/store.ts` — **pre-existing bug fixed in
  seam**: `const getPosture = client.getChatPosture` detached the method, so
  `this.getJSON` was undefined and the CP-56 boot-time posture restore always
  threw `TypeError` — swallowed by the catch, and worse, `withRetry` treated it
  as a connection error and burned 10×1.5s on every launch. Now calls
  `client.getChatPosture()` bound.
- `apps/desktop-flowpilot/src/components/ChatBootOverlay.tsx` — new overlay
  scoped to `.workspace-main` (position absolute, inset 0): centered card with
  live step checklist (✓ done / spinner active / · pending), error + Retry on
  failure, and a hint that Settings/system controls stay usable.
- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx` — mounts the
  overlay in `<main>` whenever boot isn't `ready`, covering both the chat and
  orchestration-board views for consistency (the board reads the same
  projects/agents pipeline and would otherwise sit silently empty).
  Timeline/ChatInput stay mounted underneath (drafts, provider fallback
  effects settle before lift).
- `apps/desktop-flowpilot/src/App.tsx` — kicks `loadProjects()` when phase
  becomes `authenticated` so boot progresses while the user sits in Settings
  (deduped via `loadProjectsInFlight` with Navigator's mount call).
- `apps/desktop-flowpilot/src/styles.css` — `.chat-boot-*` styles; `.term-panel`
  gets `position:relative; z-index:40` so an open terminal dock keeps
  streaming above the overlay (z 30).

## UX contract

- Loading is visible end-to-end: splash → bootstrap card → boot overlay → live UI.
- Non-blocking by scope: Settings tab, Navigator, SystemControls
  (restart/shutdown), accounts panel, and the terminal dock all stay usable —
  only the chat column is dimmed.
- Honest failure: a boot-critical failure shows which step failed + Retry
  instead of a silently dead composer.
- Zero ready-account false positives: the gate closes on data *loaded*, not on
  "an account is connected" — a user with no connected provider still gets the
  normal connect affordances, not an infinite loader.

## Tests

- New `apps/desktop-flowpilot/src/state/store.chatBoot.test.ts` — 5 tests:
  ready on success (with `globalThis.fetch` stubbed through the admin layer),
  failed on critical error, retry re-locks then readies, non-critical skills
  failure still readies, post-ready refresh never re-locks.
- `tsc --noEmit` clean; `store.chat-mode-persist`, `store.chat-posture-restore`,
  `Navigator.localOnly`, `runUpdates` — all green.
- Full `state/` suite: 388/397 pass; the 9 failures reproduce identically on
  the pre-change baseline (localStorage env + replay-ordering) — zero
  regressions from this change.
- Provider parity: provider-agnostic — the gate reads `localProviders` /
  `providerAccounts` sets; no provider branches touched.

## Honest gaps

- Overlay copy is English, consistent with the rest of the desktop shell.
- GitNexus `impact` on `loadProjects`/`ChatWorkspace` returned LOW risk (the
  index under-reports zustand call sites; callers verified by grep:
  `Navigator.tsx` mount effect + new `App.tsx` effect).

## Prior CA not undone

- CA-703 chatMode localStorage restore inside `loadProjects` — untouched and
  still covered by `store.chat-mode-persist.test.ts`.
- CP-56 posture restore intent is now actually honored (it never ran before —
  see the detached-method fix above).

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: ad-hoc-ux
change_type: bugfix
summary: chat-scoped boot overlay until workspace data lands + pre-React splash; fixes detached getChatPosture that silently broke CP-56 boot posture restore and burned 15s in withRetry
# --->8---
