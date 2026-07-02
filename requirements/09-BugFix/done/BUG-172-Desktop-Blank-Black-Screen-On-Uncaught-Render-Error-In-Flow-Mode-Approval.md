# BUG-172: Desktop Blank/Black Screen On Uncaught Render Error In Flow Mode Approval

## Metadata

- Document ID: `BUG-172`
- Title: `Desktop Blank/Black Screen On Uncaught Render Error In Flow Mode Approval`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-168-Flow-Step-Timeline-Sidebar-Hidden-During-Approval-Or-Question.md`, `requirements/09-BugFix/done/BUG-169-Spawned-Agents-Not-Visible-In-Agents-Panel-Flow-Mode.md`, `requirements/09-BugFix/done/BUG-170-Workflow-Flow-Mode-History-Unresumable-After-Server-Restart.md`, `change-audit/CA-206-agents-panel-refresh-merge-race.md`, `change-audit/CA-207-workflow-flow-mode-resume-after-restart.md`, `change-audit/CA-209-desktop-error-boundary-and-approval-error-handling.md`
- Replaces: `none`
- Tags: `agent-flow-engine, ui, desktop, flow-mode, yolo, resilience, regression`

## AI Quick View

### Summary

- User starts a "review-loop" Flow Mode run with YOLO on; when a `waiting_approval` pause appears and they click Approve, the whole desktop window goes fully blank/black — only the native Electron/OS menu bar (File/Edit/View/Window/Help) survives.
- The app's runner-side gate/prompt logs (from the user's report) show nothing abnormal at the moment of approval — `violations=0`, no server error — so the failure is on the desktop renderer side, not the runner.
- Confirmed structural cause: `apps/desktop-flowpilot/src` had **no React error boundary anywhere**. Any uncaught exception thrown while rendering the state churn that follows an approval (step-runtime refresh, agent-graph refresh, orchestration stream resuming) unmounts the entire React tree. The root `<div id="root">` is then empty, so only the body's dark theme background (`--bg: #0e1117`, near-black) shows through — exactly matching the reported black screen, with the native menu bar unaffected since it isn't part of the React tree.
- No specific throwing line could be pinpointed without a live repro (the user's log excerpt is runner-side only, no renderer devtools console/stack trace was captured), so the fix targets the missing safety net directly rather than a single hypothesized null-deref.

### Current Ask

- Stop a render exception from ever presenting as an unrecoverable blank screen with zero diagnostic information.
- Harden the approval/answer submission path itself so a failed `submitApproval`/`answerQuestion` call surfaces as a visible error instead of an unhandled promise rejection.

### Key Decisions

- `V-1` Wrap the app root in a React `ErrorBoundary` so any render-time exception, anywhere in the tree, renders a visible "Something went wrong" card with the error message and a Reload button instead of unmounting to a blank screen.
- `V-2` `store.approve()` / `store.answer()` now catch failures from `client.submitApproval` / `client.answerQuestion` and push a `system`/`error` timeline entry plus `status: "failed"` (mirroring `sendPrompt`'s existing error handling), instead of leaving an unhandled rejection during YOLO's rapid step transitions.

### Constraints

- The `ErrorBoundary` is a pure safety net (standard React `getDerivedStateFromError`/`componentDidCatch` class component) — it does not change any existing render output when nothing throws.
- Did not attempt to reproduce the exact throwing component/line: no runner + provider account was available in this environment, and the user's provided log is runner-side (gate/prompt logs), not a renderer devtools stack trace. If the crash recurs, the new boundary will now show the actual error message and component stack (via `console.error` in `componentDidCatch`), which is the fastest path to a precise root cause.
- GitNexus MCP tools were not available in this session to run the mandated impact analysis before editing `store.ts`; proceeded with careful manual inspection instead (both edits are additive — a new component and added `try/catch` blocks — and do not change existing function signatures or call sites).

### Open Questions

- What exactly throws during the post-approval render cascade in Flow+YOLO mode remains unconfirmed. The user should reproduce with Electron DevTools open (or check the renderer console) next time; the new `ErrorBoundary`'s fallback card and `console.error` log will capture the real error and component stack instead of a silent blank screen.

### Source Refs

- `apps/desktop-flowpilot/src/main.tsx` (React root mount — previously unguarded)
- `apps/desktop-flowpilot/src/App.tsx` (renders `ChatWorkspace`/`OrchestrationBoard`/`FlowTimelineSidebar` under the authenticated view)
- `apps/desktop-flowpilot/src/state/store.ts:1125` (`approve`), `:1148` (`answer` after fix, previously `:1141`)
- `apps/desktop-flowpilot/src/styles.css:2` (`--bg: #0e1117` — the near-black body background that shows through an unmounted root)
- User-provided runner log excerpt: `[gate] ... violations=0 gateMode="enforce"` immediately following the approval, with no server-side error, isolating the failure to the desktop renderer.

## 1. Issue Summary

Starting a "review-loop" workflow in Flow Mode with YOLO enabled, then clicking Approve on a pending approval card, causes the entire FlowPilot Desktop window to go blank/black. Only the native window chrome (menu bar) remains; no content, no error message, no way to recover short of restarting the app.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: FlowPilot Desktop (Electron + React 19), local runner at `http://127.0.0.1:4318`, a Flow Mode ("review-loop") run with YOLO enabled.
- reproduction steps (as reported):
  1. Start a review-loop run in Flow Mode with YOLO on.
  2. Let the run reach a point that still requests approval (task prompt: "add input validation to the parseUserID function").
  3. The desktop shows a native "Approval required" notification; click Approve.
  4. The window content goes fully black; only File/Edit/View/Window/Help remain visible.
- frequency: reported as reproducing on this run; exact trigger conditions (timing/race) not isolated in this session — could not be locally reproduced without a runner + provider account.

## 4. Expected vs Actual

- expected: approving resumes the run and the UI keeps rendering (chat, step timeline, agents panel) as the turn continues.
- actual: the entire window content unmounts to a blank/near-black screen with no error surfaced anywhere.

## 5. Impact

- users affected: anyone running Flow Mode with approvals (YOLO or not — YOLO simply makes the post-approval render cascade happen fastest/most often).
- workflows affected: Flow Mode / review-loop runs; the run itself may still be progressing server-side, but the desktop becomes unusable and must be restarted.
- severity: high — total loss of UI with no recovery path and no diagnostic information.

## 6. Root Cause

- hypothesis: a render-time exception in one of the components that re-render immediately after an approval resumes a Flow Mode run (step-timeline sidebar, agents panel, orchestration board), likely provoked by a state field that is transiently absent/undefined during the rapid succession of updates YOLO produces.
- confirmed cause (structural, not the specific throw site): `apps/desktop-flowpilot/src` has no `ErrorBoundary` anywhere in the component tree (confirmed via full-repo search). `main.tsx` mounted `<App />` directly under `React.StrictMode` with nothing to catch a render exception. React's default behavior on an uncaught render error is to unmount the whole tree from the nearest root; with no boundary, that root is the top-level `createRoot(...).render(...)` call, so the *entire* app unmounts. The body's CSS background (`--bg: #0e1117`, `styles.css:2,36`) is then the only thing left visible — a near-black fill — while the native Electron/OS menu bar survives because it is not part of the React DOM. This exactly matches the reported symptom.
- evidence: grep for `ErrorBoundary` across `apps/desktop-flowpilot/src` returned zero matches before this fix; `styles.css:2` defines `--bg: #0e1117`; `store.approve()` (pre-fix, `store.ts:1125-1139`) had no `try/catch` around `await get().client.submitApproval(...)`, unlike `sendPrompt` (`store.ts:~1098-1122`) which does catch and surface errors — an inconsistency in the same file that made the approval path specifically less resilient than the send-prompt path.
- what could not be confirmed: the exact component/line whose render throws. No renderer devtools console output or stack trace was provided (the user's log excerpt is runner-side gate/prompt logging only, which shows `violations=0` and no server error at the relevant timestamp — ruling out a backend-side failure at least for that request). Reproducing this live was not possible in this environment (no runner + provider account).

## 7. Fix Strategy

- `F-1` Added `apps/desktop-flowpilot/src/components/ErrorBoundary.tsx`, a class component implementing `getDerivedStateFromError`/`componentDidCatch` that renders a visible "Something went wrong" fallback (reusing the existing `.status-shell`/`.status-card` styling) with the error message and a Reload button, and logs the error + component stack to the console.
- `F-2` Wrapped the root render in `apps/desktop-flowpilot/src/main.tsx` with `<ErrorBoundary>` around `<App />`, so any future uncaught render exception anywhere in the tree produces a recoverable, diagnosable screen instead of a silent blank one.
- `F-3` Added `try/catch` around `await get().client.submitApproval(...)` in `store.approve()` and `await get().client.answerQuestion(...)` in `store.answer()` (`store.ts`), mirroring `sendPrompt`'s existing error handling: on failure, sets `status: "failed"`, `recoverable: true`, and appends a `system`/`error` timeline entry via the existing `runErrorMessage(err)` helper, instead of leaving an unhandled promise rejection during YOLO's rapid step transitions.

## 8. Validation

- `V-1` `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-2` Started the Vite dev server for `desktop-flowpilot` and loaded the renderer in a browser preview — app boots and renders the bootstrap screen with no console errors (no runner backend was available in this environment, so it could not proceed past the bootstrap/login screen to reach a live Flow Mode run).
- `V-3` Not executed: a live reproduction of the original black-screen symptom (approve a Flow+YOLO run's pending step) to confirm the `ErrorBoundary` actually catches the specific exception, since no runner + provider account was available in this environment. The user should retry the original repro; if it recurs, the fallback card's error message and the `componentDidCatch` console log will pinpoint the exact throwing component for a precise follow-up fix.
- `V-4` No automated unit test exists for `ErrorBoundary` (no render-test harness is set up for `apps/desktop-flowpilot` components, consistent with prior UI-only bugfixes in this area, e.g. BUG-156/158/159/168).

## 9. Regression Guard

- tests: none added (no component render-test harness in this package); typecheck is the existing guard for this package's TypeScript correctness.
- alerts: none.
- audit checks: recorded in `change-audit/CA-209-desktop-error-boundary-and-approval-error-handling.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this is an additive resilience fix, not a change to Flow Mode's intended behavior.
- notes left unchanged on purpose: the exact throwing component/line is intentionally left as an open question (see AI Quick View) rather than guessed at; the boundary itself is the durable fix regardless of which component eventually throws.
