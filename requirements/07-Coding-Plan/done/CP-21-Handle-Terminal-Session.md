# Coding Plan 21: Handle Terminal Session Lifecycle

## Context

FlowPilot's local runner manages long-lived AI provider processes (Codex, Claude, Gemini)
as child processes connected via stdin/stdout pipes (`LiveSession`). Each session is stored
in-memory in the runner's `sessions` map and persisted to the `workflow_run_sessions`
Supabase table.

### How sessions are created and used

1. `POST /sessions/start` → spawns `codex mcp-server` as a child process, performs MCP
   `initialize` handshake, stores the `LiveSession` in-memory keyed by a `processKey`,
   saves to `workflow_run_sessions` with `status = active`.
2. `POST /sessions/message` → looks up the in-memory session by `processKey`, sends
   `tools/call` (first message: `codex` tool, subsequent: `codex-reply` with `threadId`).
3. `POST /sessions/close` → closes stdin pipe, waits up to 5s, kills child process,
   removes from in-memory map, marks DB row as `completed`.

The `threadId` (Codex's conversation thread identifier) is synced to Supabase after every
message via `syncWorkflowRunSessionProviderSessionId()`.

Important constraint: Codex thread state stored under `~/.codex/` is local to a machine.
That means a solution that only reuses `threadId` is not sufficient for machine handoff.
Example: the user sends message 1 and 2 on PC A, then wants to continue on PC B. The
conversation must still be recoverable even if PC B does not have PC A's local Codex store.

---

## Issue 1 — No idle timeout: orphaned Codex processes waste memory

### Problem

`LiveSession` has no `LastUsedAt` field and the runner has no background goroutine to
detect or kill idle sessions. A session is only killed when:

- `POST /sessions/close` is explicitly called (happy path)
- The runner receives SIGINT/SIGTERM → `CleanupSessions()` kills all children

**Consequence:** If the browser tab refreshes, a workflow run is abandoned mid-step, or
the admin UI crashes, the `codex mcp-server` child process stays alive indefinitely,
consuming ~150–450 MB of RAM with no way for the runner to reclaim it automatically.

This is what causes the pile-up of Codex processes visible in Activity Monitor.

### Affected files

- `apps/local-runner/internal/runner/sessions.go` — `LiveSession` struct, no TTL field
- `apps/local-runner/internal/runner/runner.go` — `Runner` struct, no idle-sweeper goroutine

### Proposed fix

**A. Add `LastUsedAt` to `LiveSession`**

```go
type LiveSession struct {
    // ... existing fields
    LastUsedAt time.Time  // initialized on StartSession, updated on every SendMessage call
}
```

**B. Initialize and update `LastUsedAt`**

When `StartSession()` creates and registers a session, initialize:

```go
session.LastUsedAt = time.Now().UTC()
```

In `SendMessage()` (sessions.go), after acquiring `session.Mu.Lock()`, update:

```go
session.LastUsedAt = time.Now().UTC()
```

This avoids a fresh session being treated as immediately idle because of a zero-value timestamp.

**C. Add an idle-sweeper goroutine in the runner**

Start one background goroutine when the runner server boots, not once per session.
It should use a runner-owned context that is canceled during shutdown. The sweeper
ticks every N minutes and kills sessions idle longer than a configurable TTL
(e.g. 30 min).

```go
func (r *Runner) startIdleSweeper(ctx context.Context, idleTTL time.Duration) {
    ticker := time.NewTicker(idleTTL / 2)
    go func() {
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                r.sweepIdleSessions(idleTTL)
            }
        }
    }()
}

func (r *Runner) sweepIdleSessions(idleTTL time.Duration) {
    r.sessionsMu.Lock()
    var staleKeys []string
    for key, sess := range r.sessions {
        sess.Mu.Lock()
        idle := time.Since(sess.LastUsedAt) > idleTTL
        sess.Mu.Unlock()
        if idle {
            staleKeys = append(staleKeys, key)
        }
    }
    for _, key := range staleKeys {
        delete(r.sessions, key)
    }
    r.sessionsMu.Unlock()
    // kill processes outside the lock
    for _, key := range staleKeys {
        // close stdin + kill
    }
}
```

Implementation notes:

- Do not hold `sessionsMu` while waiting on child process exit.
- Collect stale session pointers while holding `sessionsMu`, delete them from the map,
  then close/kill processes outside the lock.
- Reuse the same shutdown path as `CloseSession()` / `CleanupSessions()` where possible.
- Idle eviction should only affect local runner memory/process state; do not mark the
  Supabase row completed here.

**D. Expose idle TTL as a CLI flag**

```
flowpilot runner serve --session-idle-ttl 30m
```

---

## Issue 2 — Session/process death and machine handoff lose conversation context

### Problem

When a `codex mcp-server` process dies unexpectedly (runner restart, system kill,
orphaned process killed manually), the following happens when a follow-up prompt arrives:

1. `getOrCreateSession()` finds the Supabase row with `status = active` and a stale
   `processKey`.
2. The runner no longer has this `processKey` in its in-memory `sessions` map.
3. `sendMessage()` returns "session not found or expired".
4. `sendMessageWithRetry()` (workflow-start-runtime.ts L485) catches this error,
   calls `deactivateWorkflowRunSession()` → marks DB row as `completed`, `processKey = null`.
5. It then calls `getOrCreateSession()` again → starts a **brand new** session with no
   link to the previous `threadId`.

**Consequence:** The new Codex session has zero knowledge of the previous conversation.
The user's follow-up prompt is sent to a blank context even though Codex persists thread
state on disk at `~/.codex/` and **can** resume if given the correct `threadId`.

This same failure also appears in a multi-machine handoff:

1. User sends message 1 and 2 on PC A.
2. `providerSessionId` is stored in Supabase.
3. User opens the same workflow on PC B.
4. PC B starts a fresh local runner process with an empty in-memory `sessions` map and
   no guarantee that PC A's `~/.codex/` store exists locally.
5. Reusing only the old `threadId` is not enough if the underlying local Codex thread
   store is machine-local.

### Why this matters

Codex's `codex-reply` tool accepts a `threadId` parameter. If we pass the previously
saved `providerSessionId` (= Codex `threadId`) from Supabase, a newly spawned
`codex mcp-server` process can resume the conversation. The thread history lives in
Codex's local store, not in the process's memory.

However, this only solves **same-machine** recovery if that local store is still present.
It does **not** solve cross-machine continuation by itself.

### Affected files

- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
  - `sendMessageWithRetry()` — retry path starts a fresh session, ignores saved `threadId`
  - `getOrCreateSession()` — does not attempt to reconnect with existing `providerSessionId`
- `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`
  - currently discards runner error body and reduces all failures to generic HTTP status text
- `apps/local-runner/internal/cli/root.go`
  - session endpoints currently return plain-text HTTP errors only
- `apps/local-runner/internal/runner/sessions.go`
  - runner decides whether to call `codex` or `codex-reply` from in-memory session state,
    not from the frontend handle alone

### Proposed fix

**A. Add a deterministic `session_dead` error contract end-to-end**

The current implementation relies on string matching against generic thrown errors.
Replace that with a structured runner error payload and preserve it through the web gateway.

Runner HTTP response example:

```json
{
  "code": "session_dead",
  "message": "session process exited or is no longer registered"
}
```

Required changes:

1. `apps/local-runner/internal/cli/root.go`
   - return JSON error bodies for `/sessions/message`
2. `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`
   - parse the JSON error body and throw a typed error that preserves `code`
3. `workflow-start-runtime.ts`
   - branch on the typed error code instead of string matching

**B. Add runner-owned same-machine resume**

Frontend-only override of `handle.providerSessionId` is insufficient because the runner
chooses `codex` vs `codex-reply` from `LiveSession.ProviderSessionID`.

Add an explicit resume field to session start:

```go
type AiSessionStartRequest struct {
    // existing fields...
    ResumeProviderSessionID *string `json:"resumeProviderSessionId"`
}
```

On `StartSession()` for Codex:

1. Start and initialize `codex mcp-server` normally.
2. If `ResumeProviderSessionID` is non-empty, set `session.ProviderSessionID` to that
   value before storing the session in `r.sessions`.
3. The first `SendMessage()` will then automatically choose `codex-reply` and pass the
   old `threadId`.

This is the correct place to implement same-machine resume because the runner owns the
actual live transport state.

**C. Reconnect with saved `threadId` for same-machine stale process recovery**

In `getOrCreateSession()`, when an existing DB row exists but the `processKey` is stale:

1. Detect `session_dead` from `sendMessage()` or a future explicit liveness probe.
2. Start a new `codex mcp-server` process with `resumeProviderSessionId =
   sessionRow.provider_session_id`.
3. Update the DB row with the new `processKey`.
4. Keep `providerSessionId` unchanged until Codex returns a new one.

```typescript
// Pseudo-code — reconnect path
if (sessionDead && sessionRow?.provider_session_id) {
  const newHandle = await localRunnerGateway.startSession({ ... });
  // newHandle is backed by a runner session whose internal ProviderSessionID
  // was already seeded from resumeProviderSessionId.
  handle = newHandle;
  await updateSessionRow({ processKey: newHandle.processKey });
}
```

**D. Add cross-machine continuation that does not depend on `~/.codex/`**

This is the real requirement. Same-machine `threadId` resume is an optimization, not the
primary durability strategy.

Persist a portable conversation checkpoint in Supabase/artifacts after each successful turn.
At minimum, store enough information to rebuild context on another machine:

- workflow run id
- step run id / session scope
- provider + model
- system prompt / step prompt base
- ordered user prompts
- ordered assistant outputs
- optional rolling summary to cap token growth
- artifact references produced by earlier turns

Recommended shape:

- keep `workflow_run_sessions` for live transport metadata (`processKey`, `providerSessionId`)
- add a new append-only table such as `workflow_run_session_messages` or a persisted
  conversation snapshot artifact per turn

Recovery strategy:

1. Try same-machine fast path:
   - start session with `resumeProviderSessionId`
   - send normal prompt
2. If same-machine resume is unavailable or fails:
   - start a fresh session
   - send a bootstrap prompt that reconstructs prior context from persisted conversation
     history and artifacts
   - then send the user's new follow-up prompt

Bootstrap prompt contents should be generated by FlowPilot, not by the user. It should be
clearly delimited, compact, and include prior assistant outputs as context. This is what
makes PC A → PC B continuation work.

**E. Optional liveness probe**

Before reusing a cached `processKey`, the runner may expose a lightweight liveness check.
Do not rely only on `cmd.ProcessState`, because it may still be `nil` until `Wait()` runs.
The reliable trigger is either:

- a dedicated runner liveness API, or
- structured `session_dead` returned from actual send/read failure

The implementation may add a probe later, but the typed error path is mandatory.

```go
// Example intent only; do not rely solely on ProcessState.
// A real implementation should return a typed session_dead error from
// failed write/read or explicit liveness probing.
```

### Delivery plan

`Task-02` remains the umbrella requirement for terminal session lifecycle durability.
It is only fully complete when both sub-phases below are done.

**[DONE] Task-02a — Local process lifecycle and same-machine recovery**

Scope:

- idle session sweeper
- `LastUsedAt` initialization + updates
- structured `session_dead` error contract
- same-machine resume via `ResumeProviderSessionID`
- stale-process reconnect on the same machine

Goal:

- recover when the local runner process or child provider process dies on the same machine
- avoid leaking long-lived child processes

**[DONE] Task-02b — Cross-machine continuation**

Scope:

- portable conversation checkpoint persistence
- replay/bootstrap prompt generation
- machine handoff recovery path when the original `~/.codex/` store is unavailable

Goal:

- support PC A → PC B continuation for the same workflow conversation

Delivery rule:

- shipping only `Task-02a` means same-machine recovery is improved, but `Task-02` is not complete
- `Task-02` may be marked complete only after both `Task-02a` and `Task-02b` pass

---

## Acceptance Criteria

### Issue 1 — Idle sweep

- [x] `LiveSession` has a `LastUsedAt time.Time` field initialized on `StartSession`
      and updated on every `SendMessage`.
- [x] Runner starts exactly one idle-sweeper goroutine on startup.
- [x] Sessions idle longer than the configured TTL (default: 2 hours) are automatically
      killed and removed from the in-memory map.
- [x] The idle TTL is configurable from the Settings menu and persisted in Supabase so the value
      is shared across machines.
- [x] The runner reads the shared configured TTL for session lifecycle behavior.
- [x] Users can manually terminate a live workflow session/process from the workflow-run detail
      page before the idle timeout is reached.
- [x] Killed idle sessions do NOT update Supabase (the frontend is responsible for
      detecting dead sessions on next use).

### Issue 2a — Same-machine recovery

- [x] When a session's `processKey` is stale/dead, the recovery path starts a new
      process and injects the saved `providerSessionId` inside the **runner session state**
      so Codex resumes the thread on the same machine.
- [x] The runner exposes a deterministic `session_dead` error code, and the HTTP gateway
      preserves it to the frontend.
- [x] Non-session runner/provider failures do **not** deactivate or replace a healthy
      session; reconnect is attempted only for typed `session_dead` failures.
- [x] A follow-up prompt after a same-machine process restart receives a response with
      Codex's awareness of previous conversation turns when the same machine still has the
      original local Codex thread store.
- [x] Unit tests cover: dead-session detection → reconnect → message sent with old `threadId`.
- [x] Unit tests cover: a non-`session_dead` `/sessions/message` failure is surfaced to the
      caller and does not trigger session teardown/reconnect.

### Issue 2b — Cross-machine continuation

- [x] A follow-up prompt after machine handoff (PC A → PC B) still receives a response
      with prior workflow context by replaying persisted conversation checkpoints, even if
      the original local Codex thread store is unavailable.
- [x] Portable conversation checkpoint data is persisted after each successful turn.
- [x] Recovery prefers same-machine `resumeProviderSessionId` when available, then falls
      back to bootstrap replay when it is not.
- [x] Unit tests cover: cross-machine bootstrap path when `resumeProviderSessionId`
      cannot be used.
- [x] `Task-02` is considered complete only when both Issue 2a and Issue 2b are done.

---

## Related files

| File | Role |
|------|------|
| `apps/local-runner/internal/runner/sessions.go` | `LiveSession`, `StartSession`, `SendMessage`, `CloseSession`, `CleanupSessions` |
| `apps/local-runner/internal/runner/runner.go` | `Runner` struct, sessions map |
| `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts` | `getOrCreateSession`, `sendMessageWithRetry`, `deactivateWorkflowRunSession`, `finalizeWorkflowRunSessions` |
| `apps/admin-web/src/domain/gateway/local-runner-gateway.ts` | Gateway interface: `startSession`, `sendMessage`, `closeSession` |
