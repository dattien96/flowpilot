# 04-01 — Desktop App Implementation Plan (Mock-First, then Real)

> Part of the `04` coding plan and the **single home doc for the desktop app**.
> **Part A** is the Phase 1 deliverable (mock, zero backend). **Part B** is the real
> implementation (lands once Phase 2 runner APIs exist). Shared types + the
> `ProviderEvent` union live in `04-Detailed-Coding-Plan.md`.

## Why two parts (mock-first)

Build the shell against mock data first so the UX is clickable in days and the
client↔runner contract is **locked before any Go work**. The same React renderer is
reused unchanged for the real backend — only the transport behind `RunnerClient`
swaps. Part B then wires that same shell to the real runner.

---

## Part A — Phase 1: Mock MVP (zero backend)

### Scope
- **Electron scaffold** `apps/desktop-flowpilot/`: `electron/main.ts`,
  `electron/preload.ts`, React renderer (`src/`).
- **`RunnerClient` interface** (the contract) + a **`MockRunnerClient`** behind it.
  The renderer only ever talks to `RunnerClient`.
- **UI surfaces:** project/workflow/step selector, chat input, streaming output, run
  timeline, **approval card**, **structured question card** (see "The question
  card" below), file-change rows, `/` command + **multi-skill picker** (attach one
  or more skills per turn), tool/MCP activity rows, run status.
- **Sidebar system controls:** Open Admin Web (default browser → `http://localhost:3002`),
  Restart system, Turn off system. Restart/shutdown map to the runner's
  `POST /system/restart` / `POST /system/shutdown` (mirrors admin-web's
  `local-runner-gateway`); in Part A they are stubs on `RunnerClient`.
- **Mock data + scenarios** (JSON fixtures, streamed on a timer): `normal`,
  `approval-required`, `question-required`, `tool-heavy`, `file-changes`, `failed`,
  `reconnect/replay`. A dev-only scenario switcher triggers each.

### The contract (key output)
```ts
interface RunnerClient {
  listProjects(): Promise<Project[]>;
  listWorkflows(projectId: string): Promise<Workflow[]>;
  listSteps(workflowId: string): Promise<Step[]>;
  startRun(input: StartRunInput): Promise<RunHandle>;
  resumeRun(runId: string): Promise<RunHandle>;
  sendTurn(input: TurnInput): AsyncIterable<ProviderEventDTO>; // streaming
  submitApproval(approvalId: string, decision: string): Promise<void>;
  answerQuestion(questionId: string, choice: string | string[]): Promise<void>;
  listArtifacts(runId: string): Promise<Artifact[]>;
  listSkills(provider: string): Promise<ProviderSkill[]>;
  restartStack(): Promise<void>;   // POST /system/restart  (Part B)
  shutdownStack(): Promise<void>;  // POST /system/shutdown (Part B)
}
// TurnInput carries selectedSkills?: SkillSelection[] (multi-skill).
```
`MockRunnerClient` and (Part B) `HttpWsRunnerClient` both implement this.

### Out of scope (Part A)
Real runner, Codex, Supabase, real IDE file open (stub `IdeBridge` with a console
log), auth, packaging.

---

## Part B — Real Implementation (after Phase 2)

When Phase 2 lands the runner's interactive APIs + normalized event stream:

- **Swap transport:** implement `HttpWsRunnerClient` against the **same**
  `RunnerClient` interface; select via config (`runnerUrl`). Renderer unchanged.
  Keep `MockRunnerClient` for offline/dev/UI tests.
- **Live event stream + replay:** subscribe to `/client/workflow-runs/:runId/events/stream`;
  on reconnect, rebuild the timeline from persisted events.
- **Approval card → runner approval bridge** (`04-04`): render `permission_required`
  (command, cwd, reason, decisions); submit decision; turn resumes.
- **Structured question card → user-interaction bridge** (`04-04`): render
  `user_question_required` (prompt + options + optional multiSelect + free-text
  "Other"); submit choice via `answerQuestion`; turn resumes. (This is the
  "popup with options" UX — see below.)
- **Real `IdeBridge`** (Electron main): detect + invoke IDE CLI — VS Code
  `code -g file:line`, Android Studio `studio path`, Xcode `xed file`.
- **Rendering polish:** short file name + tooltip/copy full path; collapse command
  output by default; final answer separated from logs/tools; run timeline grouping.
- **Config/auth** to the local runner; multi-window behavior (multiple windows on
  the same run/thread — see `04` Open Questions).
- **Packaging handoff** → `04-07` (Electron build + macOS signing/notarization via CI).

---

## The question card (your "popup with options" UX)

The confirm/question popup is the **same pause/resume bridge as approvals**,
generalized to a structured Q&A. Flow:

```text
model wants a decision
  -> calls FlowPilot `ask_user(prompt, options[], multiSelect?)`  (FlowPilot-owned MCP tool)
runner (user-interaction bridge, 04-04)
  -> persists a question record, emits `user_question_required`, BLOCKS the turn
desktop
  -> renders an options card (label + description per option, optional multi-select,
     free-text "Other")
  -> user picks -> answerQuestion(questionId, choice)
runner
  -> returns the choice as the tool result -> turn resumes with the answer
```

- **Provider-neutral:** implemented as a **FlowPilot-owned MCP tool** (`ask_user`)
  so it works for Codex, Claude, and Gemini through the existing MCP proxy — no
  dependence on a provider-specific feature.
- **Or native:** if the installed Codex app-server exposes a native
  elicitation/user-input request, map it to the same `user_question_required`
  event (verify availability; the MCP-tool path is the robust default).
- Reuses the approval machinery (pause turn → emit event → wait for client → resume),
  so it is cheap to add once `04-04` exists.
- **`ask_user` is a registered custom MCP tool, not built-in.** The model discovers
  it via `tools/list` and *decides* to call it — **best-effort**. For questions
  FlowPilot **must** ask, use the **workflow-driven** path (the runner emits
  `user_question_required` directly — deterministic). **Both render this same card.**
  See `04-04`.

---

## Definition of Done (checklist)

### Part A — Mock MVP

> Implemented in `apps/desktop-flowpilot/` (Electron + Vite + React 19 + TS +
> zustand). Verified headless via `tsc --noEmit` (clean) and `vite build` (renderer
> + `main.js` + `preload.js` all build). Items needing a live window are noted.

- [x] Electron scaffold + renderer; navigator + chat present. _(Build verified;
      live launch on Windows/macOS to be confirmed by running `npm run dev`.)_
- [x] `RunnerClient` interface + `ProviderEventDTO` union defined (`src/types/contract.ts`); `MockRunnerClient` implements it.
- [x] Selecting workflow/step + sending a prompt renders a **streamed** mock response with final answer separated from tool/file rows (`Timeline.tsx` + store fold logic).
- [x] `approval-required` scenario shows the approval card; approve resumes, deny stops (branched `onApprove`/`onDeny` scripts gated on the decision).
- [x] `question-required` scenario shows the **options card**; selecting an option resumes the mock turn (gate resolved by `answerQuestion`).
- [x] `/` menu + **multi-skill** picker work against the mock skill list; multiple skills attach as chips per turn (`ChatInput.tsx`).
- [x] Sidebar system controls: Open Admin Web (`shell.openExternal` → :3002), Restart, Turn off — restart/shutdown are `RunnerClient` stubs in Part A (`SystemControls.tsx`).
- [x] `failed` and `reconnect/replay` scenarios implemented (`reconnect` drops mid-stream then rebuilds the timeline on Reconnect).
- [x] Runs fully offline (zero backend). _(Short demo GIF: still to record by hand.)_

### Part B — Real implementation
- [ ] `HttpWsRunnerClient` implements the same `RunnerClient`; swap via `runnerUrl`, renderer unchanged.
- [ ] Live event stream renders message deltas; reconnect replays the timeline.
- [ ] Approval card round-trips with the runner approval bridge (`04-04`).
- [ ] Question card round-trips with the user-interaction bridge (`04-04`); `ask_user` returns the choice and the turn resumes.
- [ ] File rows open in the user's IDE via its CLI (`code`/`studio`/`xed`).
- [ ] Rendering polish: short name + full-path tooltip/copy; command output collapsed.
- [ ] A run driven end-to-end from the desktop produces the same artifacts/RAG as the web path.

### Review gate
- [ ] Human + AI review this checklist after the phase; every box ticked or explicitly deferred with a reason.
