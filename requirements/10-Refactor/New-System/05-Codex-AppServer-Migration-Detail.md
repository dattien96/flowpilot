# 05 - Codex App-Server Migration Detail (Code-Grounded, Final Decision)

This document grounds the abstract plan in `01`–`04` and `R2` into the **actual
Go runner code** (`apps/local-runner`), and records the **final scoping decision**:
one shared Codex app-server, a multi-workspace runner, and `cwd`-per-thread,
mirroring how the native Codex App Client works.

It exists because docs `03`/`04` are written provider-neutral and TypeScript-
flavored, but the real runner is Go, and the current code is **further along than
those docs assume**. This doc records what is already there, the confirmed Codex
model, the final decision, and the exact migration surface.

---

## Final Decision (Canonical)

- **Provider runtime:** Codex `app-server` becomes the chat execution path,
  replacing the one-shot `ExecutePrompt`.
- **Scoping:** **one shared app-server, multi-workspace runner, `cwd`-per-thread.**
  This mirrors the native Codex App Client: a single app-server process hosts many
  threads across many working directories.
  - This **supersedes** the earlier "one app-server per workspace" idea. That only
    looked equivalent because today a runner is hard-bound to a single workspace
    (1 runner = 1 workspace). To support "many projects, many chats each" in one
    client, the runner must become multi-workspace, and then one shared app-server
    is the natural fit.
  - **Account dimension (from code review):** auth (`CODEX_HOME`), proxy, and env
    are process-scoped (`getEnvForExecution`), so the shared app-server is bound to
    the **active provider account**. Changing the active account (manual, or the
    future auto-switch-on-usage-limit feature) **recreates** the app-server, and
    **threads are account-scoped** (persist the owning account; resume only while it
    is active). Precisely: one shared app-server **per active account**,
    multi-workspace via `cwd`-per-thread.
- **Session model:** **lean on Codex thread APIs**, do not build a custom session
  registry. FlowPilot workspace = Codex `cwd`; FlowPilot chat session = Codex
  thread; persist only the `(workspace, threadId)` mapping for list/resume.
- **Foundation:** build the Codex adapter **on top of the existing `LiveSession`
  infra in `sessions.go`**, as a sibling transport to the Gemini ACP path.
- **Compatibility:** keep `ExecutePrompt` one-shot as a fallback adapter, retired
  only after app-server covers the chat workflows.
- **Interactive client:** a **cross-platform Electron desktop app** (one codebase:
  Windows + macOS + Linux), used alongside any IDE (VS Code, Android Studio,
  Xcode) — not a VS Code extension. The React webview stays IDE-agnostic; optional
  VS Code/JetBrains plugins may reuse it later. See "Interactive Client Strategy".
- **Single backend:** the **Go runner owns all logic** and calls Supabase directly;
  the orchestration that lives in the admin-web server tier today
  (`workflow-start-runtime.ts`) is **ported into the runner**. Web UI + Desktop are
  thin clients. There is no separate orchestration service. (See `03` / `04-05`.)
- **Approval model:** **YOLO is the single source of truth (SSOT)** for approval
  posture. One resolved YOLO value drives **both** the FlowPilot runner policy and
  the Codex thread/turn config (sandbox + approval mode), consistently. This
  retires the always-on `default_tools_approval_mode = "approve"` hack. See the
  "YOLO As SSOT" section below.

---

## How Codex Models It (Confirmed Against Codex App-Server Docs)

Source: Codex app-server docs (`https://developers.openai.com/codex/app-server`).

Codex has two runtime primitives, and FlowPilot's concepts map directly onto them:

| FlowPilot concept | Codex primitive | How |
|---|---|---|
| Project / workspace | **`cwd`** | passed at `thread/start`, overridable per turn |
| Chat session | **thread** | unique `thread.id`, own JSONL log on disk |
| Many projects | many `cwd` values | nothing special — different cwds |
| Many sessions per project | many threads sharing a `cwd` | start as many threads as needed |
| Reopen a chat later | `thread/resume(id)` | reloads the JSONL log into memory |
| List chats | `thread/list` | paginated, filterable (e.g. by cwd) |
| Preview without opening | `thread/read(id)` | reads stored data without resuming |
| Archive / restore | `thread/archive` / `thread/unarchive` | move log to/from archived storage |
| Branch a chat | `thread/fork` | new thread, keeps history |
| Run a turn | `turn/start` | add user input, begin generation, stream events |

Key facts:

- **One app-server process hosts many threads concurrently.** Each thread is
  independent and carries its own `cwd`. Concurrent-thread isolation is Codex's
  normal mode — this answers the earlier open question about whether one process
  can host multiple workspaces safely.
- **`cwd` is the workspace binding.** There is no heavyweight "workspace" object in
  Codex; "projects" are a UI grouping over `cwd`.
- **Threads persist as JSONL logs on disk.** Resume reloads them. So resume needs
  both the stored `thread.id` **and** the local thread log on the same machine.

⚠️ Verify against the installed Codex build: exact param placement (`cwd` on
`thread/start` vs `turn/start`) and availability of `thread/list` / `thread/read`.
Protocol can drift across versions.

---

## YOLO As SSOT (Approval Model)

**Real safety = `permission_required` (app-server) + Codex sandbox + FlowPilot
policy, configured together.** YOLO is the single value that configures all three
consistently, resolved per workflow run/step and applied per Codex thread/turn.

This supersedes the prior model (documented in
`requirements/08-Task/todo/Task-032-Document-Proxy-Yolo-And-Provider-Approval-Config.md`)
where Codex was *always* `default_tools_approval_mode = "approve"` regardless of
YOLO and the real rule lived only in the proxy. Under app-server, YOLO drives the
Codex config directly because FlowPilot can answer approvals programmatically — so
gating no longer hangs.

### The rule

| YOLO | Codex sandbox | Codex approval mode | FlowPilot runner | Proxy MCP |
|------|---------------|---------------------|------------------|-----------|
| **true** | full-access | never (auto-run) | auto-approve any `permission_required` | auto-approve policy-allowed |
| **false** | workspace-write | on-request (emit `permission_required` for dangerous ops) | show approval card, require decision | require approval |

### Why this requires app-server

- YOLO=false + old one-shot model → Codex prompts → **hang** (the reason the
  `="approve"` hack existed).
- YOLO=false + app-server → Codex emits `permission_required` → FlowPilot answers
  it (UI card or policy) → **no hang, real gate.**

So the SSOT model depends on the app-server migration; it was not safely possible
before.

### Scoping nuance

YOLO is **not global to the shared app-server.** It is resolved per workflow
run/step (existing per-step YOLO policy, Task-030/031) and carried on
`ProviderTurnInput.yoloMode`, then mapped onto the Codex thread/turn sandbox +
approval config. Two threads in one app-server may run at different YOLO levels —
the posture rides the turn, not the process.

### YOLO=true is a posture, not safety

Because YOLO=true now disables **every** gate (full-access + never-approve), it
must be:

- explicit and visible per run/step (never a silent default),
- audited — record that the run executed under YOLO=true with gating disabled, so
  the audit trail explains why no `permission_required` events exist.

YOLO is a posture selector, not a claim of safety.

---

## Interactive Client Strategy

The interactive client is a **cross-platform Electron desktop app** — not a VS Code
extension — so one client works alongside every IDE (VS Code, Android Studio,
Xcode) instead of being tied to one editor's plugin platform.

### Why desktop, not a VS Code extension

- A VS Code extension only runs in VS Code; Android Studio (IntelliJ platform) and
  Xcode would each need a separate, incompatible plugin.
- A single Electron app ships Windows + macOS (+ Linux) from one codebase and runs
  next to any IDE.

### Layering

```text
        React webview (chat UI)  — IDE-agnostic, talks only to the runner
                 |  HTTP / WS
   +-------------+--------------+
Electron     (later) VS Code  (later) JetBrains
desktop app   plugin shell     plugin shell
   +-------------+--------------+
                 |
   Go Runner (single backend: workflow logic, app-server, finalize, Supabase)
                 |
            Codex app-server
```

- The **Go Runner is the single backend** — it owns all workflow logic and calls
  Supabase directly (see `03` topology + `04-05`). Clients stay thin.
- **React webview** holds the chat UI and talks only to the runner over HTTP/WS.
  Keep it free of Electron/IDE specifics so a future VS Code/JetBrains plugin can
  reuse it.
- **Electron main process** provides window lifecycle, auto-update, and the
  **IDE-CLI bridge** that opens files in the user's editor.

### File open across IDEs

The desktop app is not inside the editor, so "open this changed file" shells out to
the IDE's CLI launcher: VS Code `code -g file:line`, Android Studio `studio path`,
Xcode `xed file`. Small per-IDE glue in the Electron main process, not a rewrite.

### Build & distribution

- One codebase → Windows + macOS (+ Linux).
- **macOS builds require a Mac** for code-signing + Apple notarization; use CI with
  Windows and macOS runners (e.g. GitHub Actions) — one push produces both signed
  installers.
- Mobile (iOS/Android) is out of scope for this client.

### Optional IDE plugins later

A VS Code extension or JetBrains plugin (Android Studio) can come later as a
tighter-integration upgrade: a thin shell hosting the same React webview (JetBrains
via a JCEF panel) implementing the same `RunnerClient` contract — client work, not a
re-architecture, because the runner is shared.

---

## Verified Current State

The runner is a Go service (`apps/local-runner`), launched once by the supervisor
(`scripts/supervisor.js:297`, `go run ./cmd/flowpilot runner serve`). There are
**two separate provider execution paths**, scoped differently.

### Path 1: Chat execution = one-shot subprocess (the pain)

`Runner.ExecutePrompt` (`apps/local-runner/internal/runner/runner.go:852`) matches
the `codex prompt xxx` model:

```go
cmd := exec.CommandContext(execCtx, binary, args...)   // runner.go:913
cmd.Stdin  = promptFile                                 // prompt piped from a file
cmd.Stdout = stdoutFile                                 // raw output dumped to file
cmd.Stderr = stderrFile
cmd.Dir    = workspace                                  // runner.go:936
runErr := cmd.Run()                                     // spawn, wait, exit
```

Consequences (confirmed root cause of the reported pain): **no lifecycle, no
streaming, no approval interception, raw mixed output.** One process per call.

### Path 2: Session runtime = already long-lived + JSON-RPC + streaming

`apps/local-runner/internal/runner/sessions.go` already implements a long-lived
process + JSON-RPC session model, currently wired for Gemini ACP / MCP proxy:

| Symbol | Location | Role |
|---|---|---|
| `LiveSession` struct | `sessions.go:32` | process handle: `Cmd`, `Stdin`, `StdoutScanner`, `ProviderSessionID`, `ProcessKey`, `Status`, `IdleTTL`, `InFlight` |
| `Runner.StartSession` | `sessions.go:448` | spawn process, set `cmd.Dir`, run handshake, store in `r.sessions` |
| `Runner.SendMessageWithCallback` | `sessions.go:653` | send a turn and **stream** events via `SessionStreamCallback` |
| `Runner.CloseSession` | `sessions.go:1089` | tear down process/session |
| `Runner.ListSessions` | `sessions.go:1139` | enumerate live sessions |
| `writeJsonRpcRequest` / `readJsonRpcMessage` / `readJsonRpcResponseWithHandler` | `sessions.go:120-157` | JSON-RPC framing over stdio |
| `geminiACP*Params` | `sessions.go:265-283` | Gemini ACP method payloads |

Important: per-turn `cwd` already flows through — `StartSession` reads
`req.WorkingDirectory` and only falls back to `r.workspace` when empty
(`sessions.go:466-468`); `ExecutePrompt` does the same. So Codex's
"cwd-per-thread" model **matches the existing request shape**.

### Scoping reality (today)

| Layer | Current scoping |
|---|---|
| Runner process | **one per workspace** (`Runner.workspace`, `runner.go:121`, `New()` `runner.go:129`) |
| Chat (`ExecutePrompt`) | one-shot **per prompt call**, `cmd.Dir = workspace` |
| Session runtime (`LiveSession`) | **per-run** (`processKey`, `sessions.go:515`), used for ACP/MCP, not chat |

The runner being single-workspace is exactly what the final decision changes.

---

## Why "one shared app-server, multi-workspace, cwd-per-thread"

Counting app-servers for a developer with 3 projects (`/projA`, `/projB`,
`/projC`):

```
Today (single-workspace runner, 1 runner = 1 workspace):
  runner(/projA) ── app-server      "per runner" == "per workspace" == 1 each
  runner(/projB) ── app-server      (they only look equal because runner is 1:1)
  runner(/projC) ── app-server

Final (multi-workspace runner, mirrors Codex App Client):
                 ┌─ cwd /projA  (threads…)
  runner ── 1 app-server ─┼─ cwd /projB  (threads…)      one app-server total
                 └─ cwd /projC  (threads…)
```

To get the Codex-App-Client experience (all projects and their chats in one
client, one `thread/list`), the runner must hold many workspaces and route `cwd`
per thread, and one shared app-server multiplexes them — which is exactly Codex's
own design.

---

## Gap Analysis: LiveSession Today vs Final Decision

| Capability | Status in `sessions.go` | Work needed |
|---|---|---|
| Spawn long-lived process, set cwd | ✅ `StartSession` | add Codex `app-server --listen stdio://` branch; one shared process |
| JSON-RPC framing over stdio | ✅ `writeJsonRpcRequest` / `readJsonRpc*` | reuse as-is |
| Streaming turn output | ✅ `SendMessageWithCallback` + `SessionStreamCallback` | reuse; map Codex notifications |
| Protocol handshake | ✅ (Gemini ACP) | add Codex `initialize` |
| Thread lifecycle | ⚠️ custom per-run session map | replace with Codex `thread/start` / `thread/resume` / `thread/list` / `thread/read` / `thread/archive` |
| Send a turn | ✅ (ACP `session/prompt`) | add Codex `turn/start` with optimized prompt |
| Normalized event schema | ❌ none | define `ProviderEvent` + Codex event mapper |
| Approval (`permission_required`) | ❌ none | new approval bridge + resume-after-decision |
| Resume by thread id | ⚠️ `ProviderSessionID` stored | persist `(workspace, threadId)`; `thread/resume` |
| Multi-workspace runner | ❌ single `r.workspace` | route `cwd` per thread; one shared app-server |
| Lifecycle/failure states | ⚠️ `Status` string exists | formalize state set + recovery rules |

---

## Work Items (Grounded in Real Symbols)

### W1 — Multi-workspace runner + one shared Codex app-server
- Stop treating `Runner.workspace` (`runner.go:121`) as *the* project; treat the
  per-thread `cwd` as authoritative. Keep `r.workspace` only as a default.
- Start **one** `codex app-server --listen stdio://` per runner via
  `commandContextFn`, owned by the `Runner` and reused across all workspaces and
  threads.

### W2 — Codex JSON-RPC methods
- Add Codex payload builders alongside `geminiACP*Params` (`sessions.go:265`):
  `codexInitializeParams`, `codexThreadStartParams` (with `cwd`),
  `codexThreadResumeParams`, `codexThreadListParams`, `codexThreadReadParams`,
  `codexTurnStartParams`, `codexApprovalDecisionParams`.
- Reuse `writeJsonRpcRequest` / `readJsonRpcResponseWithHandler` for handshake and
  the streaming read loop.

### W3 — Thread API mapping (replaces the per-run session map)
- Map FlowPilot workspace → Codex `cwd`, chat session → Codex thread.
- Persist only `(workspace, threadId)` (plus model/status) so `thread/list` and
  `thread/resume` work after restart. Codex owns thread lifecycle and persistence.
- Back "list chats in a project" with `thread/list` filtered by `cwd`, "reopen"
  with `thread/resume`, "preview" with `thread/read`.

### W4 — Normalized events + Codex event mapper
- Define the normalized `ProviderEvent` union (per `04`) as Go types.
- Add a `CodexEventMapper` converting Codex `turn`/`item` notifications into
  `ProviderEvent`, emitted through the existing `SessionStreamCallback`.
- This fixes "raw output": final answer arrives as `message_completed` /
  `turn_completed`, separate from tool/log events.

### W5 — Approval bridge (`permission_required`) + YOLO SSOT
- Add a **YOLO resolver** that maps the per-turn YOLO value to the Codex
  thread/turn config (sandbox + approval mode) per the SSOT table, and to the
  runner approval behavior. Drives both layers from one value; retires the
  always-on `="approve"` hack.
- Map Codex approval requests to `permission_required`, persist an approval
  record, **pause the turn**, stream to the client; on decision, validate against
  FlowPilot policy, send the Codex decision back, resume the turn.
- When YOLO=true: configure Codex full-access + never-approve (no
  `permission_required` emitted); runner auto-approves; record gating-disabled in
  the audit trail.
- Pending approval state lives in the runner (survives client reconnect).
- **Boundary:** pair with Codex sandbox + approval config for real
  dangerous-command safety. App-server gives the gate; sandbox gives enforcement.

### W6 — Migrate the chat path
- Repoint chat from `Runner.ExecutePrompt` (one-shot) to a Codex-thread `turn/start`
  via `SendMessageWithCallback`.
- Keep prompt optimization **before** the turn is sent (unchanged guarantee).
- Trace and update callers of `ExecutePrompt` (workflow engine + HTTP route
  `root.go:1239`).

### W7 — Lifecycle & failure states
- Formalize `LiveSession.Status` into the `03` state set:
  `starting / ready / running / waiting_for_approval / interrupted / failed /
  completed / closed`.
- Recovery rules: process dies before turn → fail + retry; stream disconnect →
  fail session, keep events; finalizer fails after `turn_completed` → keep turn,
  retry finalize; pending approval + reconnect → reload from runner.

### W8 — Keep one-shot fallback
- Leave `ExecutePrompt` in place as a compatibility adapter until app-server covers
  required chat workflows and approvals.

---

## Resume Semantics (Concrete)

- Resume requires **both** the stored `thread.id` **and** the local Codex thread
  JSONL log on the same machine.
- After reboot: start the shared app-server, then `thread/resume(thread.id)` to
  reload the conversation.
- An **in-flight turn is not resumable** — it is re-sent.

---

## Turn Finalizer (Unchanged Guarantee)

After normalized `turn_completed`, run the provider-neutral finalizer (per `04`):
final-response artifact, changed-files/diff snapshot, summary, Supabase RAG, step
status. Finalizer failure must not erase the completed turn.

---

## Acceptance Criteria

- One shared Codex app-server serves multiple workspaces via `cwd`-per-thread.
- `thread/list` filtered by `cwd` returns a project's chats; `thread/resume`
  continues one after a runner restart.
- A chat turn streams `message_delta` → `message_completed` separate from logs.
- With YOLO=false, a dangerous command surfaces as `permission_required`; approve
  resumes, deny **blocks** the command; the decision is persisted.
- With YOLO=true, Codex is configured full-access + never-approve, no
  `permission_required` is emitted, and the run is audited as gating-disabled.
- One resolved YOLO value drives both the runner policy and the Codex thread/turn
  config; no independent `="approve"` setting remains.
- Only `(workspace, threadId)` mapping is persisted by FlowPilot; Codex owns thread
  storage.
- `ExecutePrompt` one-shot still works as a fallback.
- Workflow control, prompt optimization, artifacts, summaries, and RAG unchanged.

## Test Plan (extends `04` cross-cutting tests)

- shared app-server launches once; two threads with different `cwd` run
  independently.
- `thread/start` → `turn/start` returns `turn_completed`; deltas stream via
  `SessionStreamCallback`.
- `thread/list` filtered by `cwd`; `thread/read` previews without resuming.
- file-change notification maps to `file_changed`.
- approval request maps to `permission_required`; decision round-trips and resumes.
- `thread/resume` after process restart reattaches with context.
- in-flight turn lost on process kill → marked failed, re-sendable.
- fallback `ExecutePrompt` path still passes existing tests.

---

## Resolved vs Open Questions

Resolved:

- **App-server scoping** → one shared app-server, multi-workspace runner,
  `cwd`-per-thread (mirrors Codex App Client). Supersedes "per workspace."
- **Session model** → Codex threads via thread APIs; FlowPilot persists only
  `(workspace, threadId)`. No custom session registry.
- **Concurrent-thread isolation** → confirmed Codex's normal mode.
- **Build vs greenfield** → build on existing `LiveSession` / JSON-RPC infra.
- **Resume** → stored id **plus** local thread log, same machine; in-flight turns
  not resumable.
- **Approval model** → YOLO is the SSOT; one value drives runner policy + Codex
  sandbox/approval per turn. Retires the `="approve"` hack. Sandbox levels:
  YOLO=true → full-access; YOLO=false → workspace-write.

Still open (verify against the real Codex build):

- Exact `cwd` param placement and availability of `thread/list` / `thread/read` in
  the installed version, and `thread/resume` guarantees across a full restart.
- Which approval decisions the installed Codex app-server version exposes.
- Extension ↔ runner auth and transport (HTTP/WebSocket/local socket).
- Multi-workspace runner: workspace registration/binding API and how clients pick
  the active `cwd`.
