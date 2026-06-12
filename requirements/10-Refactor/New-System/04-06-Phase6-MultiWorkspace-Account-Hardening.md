# 04-06 — Phase 6: Multi-Workspace, Account-Scoped App-Server, Hardening & Desktop Real-Wiring

> Part of the `04` coding plan. Hardens the runtime for real multi-project,
> multi-account use and finishes the desktop client's real integration.

## Goal

Make the runner safely multi-workspace, bind the shared app-server to the active
provider account (with recreation on switch), harden lifecycle/reconnect, and
complete the desktop client's real wiring (file open, rendering polish).

## Multi-workspace runner

- Stop treating `Runner.workspace` (`runner.go:122`) as *the* project; the
  per-thread `cwd` is authoritative.
- **Audit every `r.workspace` use** and key off the active workspace/`cwd`:
  `.env` load (`runner.go:146`), runs dir (`runner.go:872` — currently writes under
  `r.workspace`, not the request cwd), Google Drive config, artifacts.
- Workspace registration/binding API (add the `cwd`-selection endpoint to the
  `04-02` client API); clients select the active `cwd`.

## Account-scoped app-server (from code review)

- The shared app-server is bound to the **active provider account** — auth
  (`CODEX_HOME`/`HOME`), proxy, and env are process-scoped (`getEnvForExecution`
  `runner.go:3394`). `scopeKey` = active account (`accountHome` + proxy + env).
- **Constraint — one active account at a time.** Because the single shared app-server
  is bound to one account, workspaces on **different** accounts **cannot run
  concurrently**; account use is serialized via recreate. (A future option is one
  app-server *process per account*; out of scope here — state the limit explicitly.)
- Changing the active/effective account **tears down and recreates** the app-server:
  interrupt in-flight turns and mark them **recoverable (re-sendable)** — consistent
  with the resume model (turns are not silently auto-replayed) — then
  re-`ensureCodexAppServer`.
- **Auto-switch-on-usage-limit (future):** if it fires mid-turn, finish the current
  turn if the provider allows; otherwise interrupt → recoverable fail → **notify the
  user**, then switch. Never silently drop work.
- **Threads are account-scoped:** a thread's JSONL log lives under that account's
  `CODEX_HOME`. `thread/resume`/`thread/list` work only while that account is
  active; otherwise surface "switch account to access."

## Lifecycle & recovery hardening

- Formalize status set (`03`): `starting/ready/running/waiting_for_approval/
  interrupted/failed/completed/closed`.
- Recovery rules: process death → fail in-flight, keep events, lazy-restart;
  stream disconnect → fail session, keep events; finalizer fail → keep turn, retry;
  pending approval + reconnect → reload from runner; multiple clients → runner is the
  single decision authority.
- Shared-process idle/shutdown policy (per-session `IdleTTL` no longer maps 1:1).

## Desktop real-wiring

The desktop client's real implementation (swap `MockRunnerClient` →
`HttpWsRunnerClient`, real `IdeBridge`, rendering polish) is specified in **`04-01`
Part B** and completes in this phase. The runner-side prerequisites it depends on
(multi-workspace, account scope, reconnect/replay) are the sections above.

## Acceptance

- Two workspaces (different cwd) run independently; switching active account
  recreates the app-server and hides the other account's threads until switched
  back.
- Reconnect replays the timeline; pending approval reloads.
- Desktop opens changed files in the user's IDE via its CLI.

## Tests

- T-02 multi-cwd; T-15 resume after restart; T-16 reconnect/replay.
- T-43 account switch recreates the app-server, interrupts in-flight (recoverable),
  and hides the other account's threads; workspaces on different accounts cannot run
  concurrently (serialized).
- crash → in-flight turns fail and are re-sendable.

## Definition of Done (checklist)

- [ ] `Runner.workspace` demoted; every `r.workspace` use audited + keyed to active cwd (incl. runs dir `runner.go:872`).
- [ ] App-server bound to active account; account switch recreates it (interrupt in-flight + mark recoverable, not silent auto-replay) (T-43).
- [ ] **Constraint stated:** workspaces on different accounts cannot run concurrently (serialized via recreate); auto-switch-on-usage-limit mid-turn notifies the user, never drops work silently.
- [ ] Threads account-scoped; resume/list only while owning account active (T-15).
- [ ] Lifecycle status set + recovery rules; reconnect replays (T-16); pending approval reloads.
- [ ] Desktop real-wiring complete per `04-01` Part B (HttpWsRunnerClient, real IdeBridge, rendering polish).
- [ ] Two workspaces run independently (T-02); crash → in-flight turns re-sendable.
- [ ] **Review gate:** human + AI review this checklist after the phase.
