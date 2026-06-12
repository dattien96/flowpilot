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

> Implemented at the `InteractiveService` layer (`interactive_service.go`,
> `interactive_handlers.go`, `provider_event.go`, `provider_registry.go`) + the
> desktop Electron main (`apps/desktop-flowpilot/electron/main.ts`), tested via
> `phase6_test.go` + the desktop `tsc`/`vite build`. `go vet` clean; all Phase 6
> tests pass; no regressions vs the HEAD baseline.
> **Deferred to the live-Codex registry swap (06 Part D):** the OLD `runner.go`
> `r.workspace` audit (it is the live-session path being replaced) and the real
> app-server **process** teardown/recreate (`ensureCodexAppServer`) — there is no
> `codex` binary here. The account-switch *orchestration* (interrupt + recoverable +
> scoping) and the multi-workspace cwd binding are implemented + tested on the new
> InteractiveService path; they drive the process layer once it exists.

- [~] **Multi-workspace:** per-run `cwd` binding is authoritative (StartRunInput.Cwd → run → TurnRequest → admin session view); runs on different workspaces are independent (T-02). _The OLD `runner.go` `r.workspace` audit (runs dir `runner.go:872`, `.env`, GDrive config) is deferred with the live-session path it belongs to._
- [~] **Account switch:** `SetActiveAccount` interrupts in-flight turns + marks them recoverable (re-sendable, not silent auto-replay) and scopes the previous account's runs out (`409`) until reactivated (T-43). _The app-server **process** recreate (`ensureCodexAppServer`) is deferred — needs a real `codex`._
- [x] **Constraint stated + enforced:** one active account at a time → workspaces on different accounts cannot run concurrently (serialized via the active-account gate + `409`). Auto-switch-on-usage-limit (mid-turn notify, never silently drop) is documented as the future path; the never-silent-drop guarantee already holds (interrupt → recoverable fail + event).
- [~] Threads account-scoped; resume/turn only while the owning account is active (the `409 provider_account_changed` gate, P2 + T-43). _Codex `thread/resume`/`thread/list` JSONL scoping under `CODEX_HOME` is deferred to the live app-server._
- [x] Lifecycle + recovery: reconnect replays the timeline via the SSE `afterSeq`/`Last-Event-ID` cursor and pending approval/question reload from the run snapshot (P2); interrupt → recoverable fail; finalizer failure keeps the turn (P4). _(The formal status-set rename to the 03 vocabulary is left as-is to avoid breaking the desktop contract; the mapping is documented.)_
- [~] Desktop real-wiring (`04-01` Part B): `HttpWsRunnerClient` swap (earlier session), live stream + reconnect replay, approval + question round-trip, and the **real IdeBridge** (code/cursor/studio/xed) are done; short-name + full-path tooltip is in. _A live end-to-end run vs the web path is deferred with the live backend._
- [x] Two workspaces run independently (T-02); interrupt/crash → in-flight turns fail **recoverable** (re-sendable).
- [x] **Review gate:** AI review complete this pass; _human sign-off pending._
