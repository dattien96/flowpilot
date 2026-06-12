# desktop-flowpilot

FlowPilot's cross-platform desktop client (Electron + Vite + React). This is the
**Phase 1 mock MVP** described in
`requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md` — **Part A**
(zero backend; everything is mock data streamed on a timer).

## Run

```bash
cd apps/desktop-flowpilot
npm install
npm run dev          # launches Vite + Electron (the desktop window)
```

To preview the renderer in a plain browser tab (no Electron), open the Vite URL
printed in the console. The `IdeBridge` falls back to `console.log` there.

```bash
npm run typecheck    # tsc --noEmit
npm run build        # type-check + production renderer/electron build
```

## What's here (Part A)

- **`RunnerClient` contract** — `src/types/contract.ts`. The one artifact Phase 1
  locks. `MockRunnerClient` implements it now; `HttpWsRunnerClient` implements the
  same interface in Part B (renderer unchanged).
- **`MockRunnerClient`** — `src/client/`. Streams scripted `ProviderEventDTO`s;
  approval/question scenarios block on a gate that `submitApproval`/`answerQuestion`
  resolve (the same pause/resume shape the real runner uses, 04-04).
- **UI surfaces** — navigator (project/workflow/step), chat input with `/` skill
  picker, streaming timeline, **approval card**, **question (options) card**,
  tool-activity rows, file-change rows, run status.
- **Scenario switcher** (dev-only) — `normal`, `approval-required`,
  `question-required`, `tool-heavy`, `file-changes`, `failed`, `reconnect/replay`.

## Out of scope (Part A)

Real runner, Codex, Supabase, real IDE open (the `IdeBridge` is a stub that logs),
auth, packaging. All land in **Part B** / later phases.
