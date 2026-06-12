# 04-07 — Operator Docs

> How to run and configure the refactored FlowPilot (Codex app-server runner +
> desktop/web thin clients). Companion to `04-07-Migration-Notes.md`.

## Components

- **Local runner** (`apps/local-runner`, Go) — the single backend. Serves the
  interactive + admin APIs (`04-02`), hosts the provider adapters, owns approvals /
  questions / finalize, and (post-cut-over) reads/writes Supabase directly.
- **Desktop client** (`apps/desktop-flowpilot`, Electron) — thin client over the
  runner's HTTP/SSE API. Swap transport via `VITE_RUNNER_URL` (else mock).
- **Admin Web** (`apps/admin-web`) — configuration + read-only audit; thin client of
  the runner for run control.

Start everything for local dev with `just dev` (web + runner + desktop).

## Provider & account setup

- Providers live in a registry (`/admin/providers`): **Codex** is implemented;
  **Claude** and **Gemini** are placeholders (visible, disabled). A disabled
  provider cannot be selected for a run — the runner rejects it with a typed
  `UnsupportedProviderRuntimeError` (this is enforced runner-side, not just in the
  UI).
- The shared Codex app-server is bound to the **active provider account**
  (`CODEX_HOME`/`HOME` + proxy + env). **One active account at a time:** switching
  the active account (`POST /client/active-account`) tears down + recreates the
  app-server, interrupts in-flight turns (marked **recoverable / re-sendable**), and
  hides the previous account's threads until it is active again. Workspaces on
  *different* accounts therefore cannot run concurrently — account use is serialized.

## Workspaces (multi-project)

- The per-run/per-thread **cwd is authoritative**, not a single global workspace.
  Clients pass the active `cwd` on run start; two runs in different workspaces are
  independent.

## YOLO posture (read this before enabling it)

YOLO is the **single source of truth** that configures both the Codex thread and the
runner approval policy (`resolveYoloPosture`):

| YOLO | Codex sandbox | Codex approval mode | Runner approvals |
|------|---------------|---------------------|------------------|
| **off** (default) | `workspace-write` | `on-request` | dangerous commands → `permission_required`; allow/deny policy or ask the human |
| **on** | `full-access` | `never` | **gating disabled** — auto-approved, audited as gating-disabled |

- **YOLO=true disables the safety gate.** The model can run dangerous commands
  without asking. The run is audited as gating-disabled, but nothing pauses it. Only
  enable it for trusted, sandboxed work.
- **Real dangerous-command safety = `permission_required` + the Codex sandbox +
  FlowPilot policy together.** The app-server's approval channel is the gate channel,
  not enforcement by itself — the **Codex sandbox is what actually contains a command
  on disk/network**. Do not rely on approvals alone; keep the sandbox configured.
- With YOLO off, the **approval policy engine** can auto-approve an allowlist and
  auto-deny a denylist (configured in Admin Web); everything else shows the card.
  Every auto-decision still replies to the provider so it never hangs.

## Approvals & questions

- **Approval** (`permission_required`): runtime-initiated, the model cannot skip it.
- **`ask_user` MCP tool**: model-initiated, best-effort — the model may not call it.
- **Workflow-driven question**: runner-initiated at a defined step, deterministic —
  the card always appears.

All three render the same desktop card and resume the turn on answer; all are
idempotent, first-write-wins, and expire (recoverable) if unanswered.

## Desktop IDE integration

File-change rows open in the user's IDE via its CLI: VS Code `code -g file:line`,
Cursor, Android Studio `studio`, Xcode `xed`. The first one found is used.
