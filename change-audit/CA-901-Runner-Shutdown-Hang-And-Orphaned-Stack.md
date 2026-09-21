# CA-901 — Runner shutdown hang + orphaned dev-stack processes

Found during CP-70 live desktop verification (`just dev`): Ctrl+C on the
supervisor terminal and the desktop "⏻ Turn off system" button both appeared
to do nothing — runner kept serving on its port and chat kept returning
`502 dispatch_prepare_failed`.

Root causes, all fixed:

1. **`/system/shutdown` and `/system/restart` ran `CleanupSessions()`
   synchronously before responding or exiting.** Any stall inside cleanup
   (provider process mutexes, synchronous `cmd.Wait()`) wedged the endpoint
   and, via the SIGINT handler doing the same, made the runner unkillable
   through every graceful path. Both handlers now answer 202 + write
   `supervisor.cmd` first, then run cleanup asynchronously under a hard
   `shutdownCleanupBudget` (2s) and exit regardless. SIGINT cleanup is
   bounded the same way via `cleanupSessionsBounded`.

2. **`ensureDevinProcessSegmented` held `devinProcessMu` across spawn +
   `initialize` (30s timeout) + `authenticate` (90s PKCE timeout).** A
   mid-flight handshake blocked `closeAllDevinProcesses` → `CleanupSessions`
   → shutdown for up to ~2 minutes. The ensure is now split: locked
   reuse/reclaim fast path → unlocked spawn+handshake → locked CAS insert
   (concurrent winners reuse the existing handle and tear down the
   duplicate). `devinRunSessions` lazy init moved into the locked insert so
   it stays race-free. Grok (`ensureGrokProcess`) and OpenCode
   (`ensureOpencodeProcessSegmented`) share the same hold-mutex-across-
   handshake shape; they are covered by the bounded-cleanup fix in (1) but
   the same refactor remains a follow-up hardening opportunity.

3. **Desktop `shutdownStack`/`restartStack` POSTs had no timeout** —
   `postJSON` hung forever on a wedged handler, leaving the button on
   "Shutting down…" indefinitely. Both now pass `AbortSignal.timeout(10s)`.

4. **Supervisor had no SIGHUP handler.** Managed children are spawned
   `detached` (own process groups), so closing the terminal killed the
   supervisor without running `cleanupAndExit`, orphaning runner/web/desktop
   — the orphaned runner then held `dispatch.lock` flocks, which produced
   the observed `dispatch_prepare_failed` 502s for the *next* runner on the
   same chats root. `SIGHUP` now routes to `cleanupAndExit`.

5. **The IDE terminal exported `ELECTRON_RUN_AS_NODE=1`, and the supervisor
   copied it into the desktop child.** Electron therefore ran as plain Node;
   `electron.ipcMain` was undefined and the new desktop exited immediately
   with `TypeError: Cannot read properties of undefined (reading 'handle')`.
   The still-visible window was a stale Electron instance outside the active
   supervisor, which explained why fetch and lifecycle behavior appeared
   inconsistent. The supervisor now removes `ELECTRON_RUN_AS_NODE` from the
   desktop environment before spawning `npm run dev`.

6. **Desktop turns were blocked by Chromium CORS preflight.** Every
   `HttpWsRunnerClient.postJSON` request includes `X-Client: desktop`, but
   runner CORS allowed only `Content-Type`, `Idempotency-Key`, and
   `Last-Event-ID`. Health GETs therefore stayed green while every turn POST
   failed before reaching the runner with `TypeError: Failed to fetch`.
   `X-Client` is now present in `Access-Control-Allow-Headers`, protected by
   `TestWithCORSAllowsDesktopClientHeader`.

Live-verified on a test binary: `POST /system/shutdown` → `{"status":"accepted"}`
→ listener dead; SIGINT → "Shutting down local runner…" → exit. Re-ran the
full `just dev` stack with the hostile inherited variable: Electron main,
renderer and GPU helpers stayed alive instead of crashing. Posting
`/system/shutdown` then killed supervisor, Electron, runner, admin web,
Vite and LibreTranslate and released ports 4318/5174/3012/5002 in under one
second.
`go test -run Devin` passes; `go build ./...` + `go vet` clean.
Pre-existing failure (unrelated, fails on base commit):
`TestCleanupSessionsTearsDownProviderPools` — claude probe `ProcessState`
nil after cleanup; desktop `tsc` has a pre-existing
`store.chat-mode-persist.test.ts` arg-count error.

# ---8<--- flowpilot:change-ledger
feature_key: terminal-session
source_doc_id: CP-70
change_type: bugfix
summary: Fixes dev-stack shutdown hangs and desktop turn fetch failures — shutdown/restart/SIGINT now use bounded async cleanup; Devin handshake no longer holds devinProcessMu; desktop lifecycle POSTs are timed out; supervisor handles SIGHUP and strips ELECTRON_RUN_AS_NODE; runner CORS now allows the desktop X-Client header.
# --->8---
