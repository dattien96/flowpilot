# 09 — Implementation Guide: Cross-Account Chat Resume (Codex + Claude)

> **Audience:** an implementing agent (Codex) that will write the code directly.
> **Goal:** make a past chat run **re-openable and continuable**, including under a
> *different provider account* on the same PC. Built on the proven fact that provider
> session files are account-agnostic and portable ([CA-098](../../../change-audit/CA-098-spike-provider-session-portability.md)).
>
> This guide is self-contained. Every decision that could otherwise become a question
> is pre-resolved in §2–§3. Follow the phases in order. Do not redesign; implement.

Related task docs (background, do not need editing to follow this guide):
[Task-067](../../08-Task/todo/Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md),
[Task-068](../../08-Task/todo/Task-068-Desktop-History-Unified-View-Account-As-Local-File-Pointer.md),
[Task-069](../../08-Task/todo/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md) (cross-PC, deferred),
[BUG-080](../../09-BugFix/done/BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md) (the persistence layer this builds on).

---

## 0. Proven facts (do not re-investigate)

From the CA-098 spike, verified end-to-end on real accounts:

- A Codex **rollout file** (`<CODEX_HOME>/sessions/YYYY/MM/DD/rollout-<ts>-<UUID>.jsonl`) is **account-agnostic** — its session-meta payload has keys `id, timestamp, cwd, originator, cli_version, source, model_provider, base_instructions, dynamic_tools` and **no** account/auth/user/token field.
- `codex exec resume <UUID> [PROMPT]` **resolves a rollout by id straight from `$CODEX_HOME/sessions`** — no `session_index.jsonl` entry required. Copying a rollout from account A's home into account B's home and running `codex exec resume` under `CODEX_HOME=<B>` resumed it and **completed a turn** (model replied), billed to B. The **only** precondition is that account B is **validly logged in**.
- Claude sessions live at `~/.claude/projects/<cwd-hash>/<sessionId>.jsonl` and Claude already resumes via `claude --resume <sessionId>` per turn.

**Implication:** "continue chat A under account B" = *relocate the session file into the active account's home, then resume it.* No history-injection, no transcript replay.

---

## 1. Scope

### In scope (this guide)
- **Chat runs only** (`runKind == "chat"`). Workflow runs are Phase 2 (see §10).
- **Re-open + continue a past run after restart** (same account) — the foundation.
- **Continue a past run under a different account** on the **same PC** (cross-account).
- **Fallback:** when a run cannot be re-opened (provider rejects the file, or the target account is not logged in), the desktop shows the history item **greyed-out / disabled with a reason** — never a crash, never history-injection.

### Out of scope (do NOT implement here)
- **Cross-PC sync** (Google Drive + `sessions.ndjson`) → [Task-069](../../08-Task/todo/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md), Phase 3.
- **Workflow (multi-step) run resume** → Phase 2.
- Account badges / grouping / filtering in history UI — history stays **unified** (one list).
- Warm long-lived provider processes — keep the existing spawn-per-turn / app-server models.

---

## 2. Pre-made decisions (so you do not ask)

- **D-1** `run_id` is the durable identity. `provider_session_id` and `provider_account_id` are **mutable** per run — they are re-pointed when a run is resumed under a new account. The user always sees the same chat.
- **D-2** The history list is **unified**. Never filter, badge, or group by account. `provider_account_id` is internal plumbing only (locates the session file's home folder).
- **D-3** Fallback = **greyed-out disabled item with a reason string**. Reasons: `"session data not found on this machine"`, `"can't open — created by a different account that isn't signed in"`. Never inject transcript as context.
- **D-4** MVP is **chat only**. Guard every new code path on `runKind == "chat"`; for workflow runs, return the existing `run_not_found`/greyout until Phase 2.
- **D-5** Do **not** modify `fakeWorkflowStore` (it is the in-memory test double). All persistence changes go in `localFileSessionStore` and `ProviderSessionState`.
- **D-6** NDJSON schema changes must stay **backward-compatible** (`omitempty`; missing fields load as empty string).

---

## 3. The one architectural decision (resolved)

**Codex resume transport.** The Codex app-server integration (`codex_adapter.go`) starts a **fresh thread every turn** (`thread/start` → `turn/start`) and has **no `thread/resume`**. Two ways to resume an existing rollout:

- **(A) CLI `codex exec resume <id>`** — *proven in CA-098*, works today on codex 0.140.0, account-agnostic.
- (B) App-server `thread/resume` — **does not exist** in this codebase and is not known to be exposed by the app-server protocol.

> **DECISION (use this): Option A.** Implement Codex chat resume via a dedicated **CLI resume turn path** (`codex exec resume`), modeled on the existing Claude spawn-per-turn process layer (`claude_process.go`). Do not attempt to add `thread/resume` to the app-server. Fresh (non-resumed) Codex runs keep using the app-server exactly as today. Only a run being *re-opened* uses the CLI resume path for its turns.

This keeps the change additive and grounded in a verified mechanism.

### 3.1 Codex resume — two items to verify during implementation (not design forks)

Both have a safe default; neither blocks the design or requires asking back:

- **`codex exec` output format** for event mapping — confirm from `codex exec --help` on the pinned version whether a streaming / `--json` mode exists. Default if not: capture the final stdout assistant block as one `EventMessageCompleted` + `EventTurnCompleted`.
- **Which rollout is "the chat".** FlowPilot's app-server path runs `thread/start` per turn, so a multi-turn chat *may* span multiple rollout files rather than one. CA-098 verified the single-rollout case (a 28k-token multi-turn rollout resumed cleanly). Rule: Phase A-2 persists the **canonical rollout session id for the run** — the rollout whose `cwd` matches the run and is newest for it. If only one rollout exists (common), that is the handle. This affects *which id* gets persisted, not the resume mechanism.

---

## 4. Phase A — Persist the real provider session id + account id

**Problem being fixed:** today the *real* provider session handle is not durably stored.
- Claude's real session id lives only in `claudeProcessPool.real` (in-memory, lost on restart).
- `ProviderSessionState.ProviderSessionID` is set once at `createRun` to the **synthetic** id (`nextID("thread")`), never updated to the real handle.
- `ProviderAccountID` is on `ProviderSessionState` but **not written to `sessions.ndjson`**.

Without the real handle + the account that owns the files, resume is impossible after restart.

### A-1 — Add `provider_account_id` to the NDJSON record
File: `apps/local-runner/internal/runner/local_file_session_store.go`

In `ndjsonSessionRecord` add:
```go
ProviderAccountID string `json:"provider_account_id,omitempty"`
```
In `sessionRecordFrom`: `ProviderAccountID: s.ProviderAccountID,`
In `sessionStateFromRecord`: `ProviderAccountID: r.ProviderAccountID,`

### A-2 — Persist the **real** provider session id
The real id is the resume handle. Re-point `ProviderSessionState.ProviderSessionID` from the synthetic id to the real one as soon as it is known, and persist.

- **Claude:** the real session id is captured into `claudeProcessPool.setRealSession(fpSessionID, claudeSessionID)` (`claude_process.go`). Add a callback so the service persists it. Concretely:
  - Add a field to `interactiveRun`: `realProviderSessionID string`.
  - In the Claude event path that currently calls `setRealSession` (the `system/init` / `result` frame handler in `claude_adapter.go` / `claude_event_mapper.go`), also surface the real id to the service. Simplest wiring: when the turn finishes (`runTurn` → after `finishTurn`), read `pool.realSession(rs.providerSessionID)`; if non-empty and different from `rs.realProviderSessionID`, set it and persist.
  - Update `sessionStateOf` (below, A-3) to write the real id into `ProviderSessionState.ProviderSessionID` when present, else the synthetic id.
- **Codex:** the durable handle is the **rollout session id**. The app-server path does not expose it cleanly today; for MVP, capture it from the `thread/start` response if present, otherwise leave the synthetic id and rely on the rollout discovery helper in §6.3 (locate newest rollout by cwd+time as a fallback). Persist whatever id is known.

### A-3 — `sessionStateOf` writes the resume handle + account id
File: `apps/local-runner/internal/runner/interactive_service.go` (`sessionStateOf`)

```go
func sessionStateOf(rs *interactiveRun) ProviderSessionState {
    providerSessionID := rs.providerSessionID
    if rs.realProviderSessionID != "" {
        providerSessionID = rs.realProviderSessionID // resume handle, re-pointed (D-1)
    }
    return ProviderSessionState{
        RunID:             rs.id,
        ProjectID:         rs.projectID,
        WorkflowID:        rs.workflowID,
        ProviderSessionID: providerSessionID,
        ProviderKey:       rs.providerKey,
        ProviderAccountID: rs.providerAccountID,
        WorkingDirectory:  rs.workspaceCwd,
        Status:            rs.status,
        LastPrompt:        rs.lastPrompt,
        LastMessage:       rs.lastMessage,
        StartedAt:         rs.createdAt,
        UpdatedAt:         rs.updatedAt,
        RunKind:           rs.runKind,
    }
}
```
No new persist call sites are needed — `startTurn` and `runTurn` already call `persistProviderSession(sessionStateOf(rs))`. The real id now flows through automatically once A-2 sets `rs.realProviderSessionID`.

### A-4 — Tests
Extend `local_file_session_store_test.go`: assert `ProviderAccountID` and a re-pointed `ProviderSessionID` survive the restart round-trip (upsert with new values for an existing `run_id`, reload, assert last-wins).

---

## 5. Phase B — Reconstruct a run after restart (the resume foundation)

File: `apps/local-runner/internal/runner/interactive_handlers.go` (`resumeRun`),
`workflow_store.go` (interface), `local_file_session_store.go` (impl).

### B-1 — Add single-run lookup to the reader
Interface `SessionHistoryReader` (in `workflow_store.go`) gains:
```go
GetProviderSession(ctx context.Context, runID string) (ProviderSessionState, bool, error)
```
Implement on `localFileSessionStore` (read from the in-memory map populated by `loadFromDisk`):
```go
func (s *localFileSessionStore) GetProviderSession(_ context.Context, runID string) (ProviderSessionState, bool, error) {
    s.fakeWorkflowStore.mu.Lock()
    defer s.fakeWorkflowStore.mu.Unlock()
    st, ok := s.fakeWorkflowStore.sessions[runID]
    return st, ok, nil
}
```
(Implement the same method on `SupabaseWorkflowStore` for parity, querying one row by `workflow_run_id`. Optional for the non-Supabase MVP but keep the interface satisfied — add a compile-time `var _ SessionHistoryReader = ...` guard.)

### B-2 — `resumeRun` falls back to disk
Today `resumeRun` returns `run_not_found` when `s.runs[runID] == nil`. Add reconstruction:

```go
func (s *InteractiveService) resumeRun(runID string) (RunHandle, *apiErr) {
    s.mu.Lock()
    rs := s.runs[runID]
    s.mu.Unlock()

    if rs == nil {
        // Post-restart: rebuild from the persisted session (BUG-080 store).
        reader, ok := s.workflowStore.(SessionHistoryReader)
        if !ok {
            return RunHandle{}, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
        }
        st, found, _ := reader.GetProviderSession(context.Background(), runID)
        if !found {
            return RunHandle{}, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
        }
        if st.RunKind != "chat" { // D-4: chat MVP only
            return RunHandle{}, newAPIErr(http.StatusConflict, "resume_unsupported", "only chat runs can be resumed in this version")
        }
        rebuilt, e := s.reconstructRun(st) // §B-3
        if e != nil {
            return RunHandle{}, e
        }
        rs = rebuilt
    }

    if rs.providerAccountID != s.activeAccountID {
        // Cross-account: attempt relocation+resume (§6). If it cannot be made
        // resumable, surface a typed error the desktop renders as greyed-out (D-3).
        if e := s.prepareCrossAccountResume(rs); e != nil {
            return RunHandle{}, e
        }
    }

    handle := RunHandle{RunID: rs.id, ProviderSessionID: rs.providerSessionID, ProviderKey: rs.providerKey, Status: rs.status}
    if rs.runKind == "chat" {
        handle.StepID = "chat-" + rs.id
    }
    return handle, nil
}
```

### B-3 — `reconstructRun`
Rebuild an `interactiveRun` from `ProviderSessionState` and register it in `s.runs`:
```go
func (s *InteractiveService) reconstructRun(st ProviderSessionState) (*interactiveRun, *apiErr) {
    now := time.Now().UTC().Format(time.RFC3339Nano)
    rs := &interactiveRun{
        id:                    st.RunID,
        projectID:             st.ProjectID,
        workflowID:            st.WorkflowID,
        providerKey:           st.ProviderKey,
        providerSessionID:     st.ProviderSessionID, // the REAL resume handle (Phase A)
        realProviderSessionID: st.ProviderSessionID,
        providerAccountID:     st.ProviderAccountID,
        workspaceCwd:          st.WorkingDirectory,
        runKind:               st.RunKind,
        status:                st.Status,
        createdAt:             st.StartedAt,
        updatedAt:             now,
        lastPrompt:            st.LastPrompt,
        lastMessage:           st.LastMessage,
        subs:                  map[int64]chan ProviderEvent{},
        idempotency:           map[string]string{},
        resumedFromDisk:       true, // new bool flag on interactiveRun; see §6
    }
    s.mu.Lock()
    s.runs[rs.id] = rs
    // Re-seed the synthetic chat step so the next turn has a stepId (mirrors createRun).
    if seeder, ok := s.workflowStore.(workflowRunSeeder); ok {
        seeder.seed(rs.id, []RuntimeWorkflowStep{{ID: "chat-" + rs.id, StepType: "chat", Status: StepStatusPending}})
    }
    s.mu.Unlock()
    return rs, nil
}
```
Add fields to `interactiveRun`: `realProviderSessionID string`, `resumedFromDisk bool`.

---

## 6. Phase C — Provider resume wiring

### 6.1 — Account home resolution (shared)
Resolve the on-disk home for a given account so files can be located/relocated. Reuse the existing provider-accounts subsystem:
- `ListProviderAccounts()` returns account records with a `HomePath` (see `claude_adapter_test.go` usage `account.HomePath`).
- `DetectDefaultAccountHomePath(providerKey)` gives the default home.

Add a helper:
```go
// resolveAccountHome returns the filesystem home dir for an account id + provider.
// Falls back to the provider default home when accountID is "" or "default".
func (s *InteractiveService) resolveAccountHome(providerKey ProviderKey, accountID string) (string, bool) { ... }
```

### 6.2 — Locate + relocate the session file (shared)
New file `apps/local-runner/internal/runner/session_file_locator.go`:
```go
// LocateSessionFile returns the absolute path of the provider session file for a
// given session id under the given account home, or ("", false) if not found.
//   Codex:  <home>/sessions/**/rollout-*-<sessionID>.jsonl   (glob by id suffix)
//   Claude: <home>/.claude/projects/<cwd-hash>/<sessionID>.jsonl
func LocateSessionFile(providerKey ProviderKey, accountHome, sessionID, cwd string) (string, bool)

// RelocateSessionFile copies a session file into the target account's home at the
// path that provider expects (additive; never overwrites; creates dirs). Returns the
// destination path. Used for cross-account resume (D-1).
func RelocateSessionFile(providerKey ProviderKey, srcPath, targetHome, sessionID, cwd string) (string, error)
```
For Claude the `<cwd-hash>` is Claude's own hash of the working directory; compute it the same way Claude does (read it from the existing path under the source home — the hash dir already exists there — and reuse the same hash dir name under the target home).

### 6.3 — `prepareCrossAccountResume`
```go
func (s *InteractiveService) prepareCrossAccountResume(rs *interactiveRun) *apiErr {
    srcHome, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
    if !ok { return newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine") }
    src, found := LocateSessionFile(rs.providerKey, srcHome, rs.providerSessionID, rs.workspaceCwd)
    if !found { return newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine") }

    targetHome, ok := s.resolveAccountHome(rs.providerKey, s.activeAccountID)
    if !ok { return newAPIErr(http.StatusConflict, "account_unavailable", "active account home not found") }
    if !HasLocalAuthAtPath(string(rs.providerKey), targetHome) {
        return newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
    }
    if _, err := RelocateSessionFile(rs.providerKey, src, targetHome, rs.providerSessionID, rs.workspaceCwd); err != nil {
        return newAPIErr(http.StatusConflict, "session_unavailable", "could not prepare the session on the active account")
    }
    // Re-point the run to the active account (D-1) and persist.
    rs.providerAccountID = s.activeAccountID
    _ = s.persistProviderSession(sessionStateOf(rs))
    return nil
}
```
The typed error `code`s above are what the desktop maps to the greyed-out reason (D-3, §7).

### 6.4 — Claude resume turn
Claude already resumes per turn. The only change: after reconstruction/relocation, the next turn must `--resume` the **real** id. Seed the pool so the existing path uses it:
- When a run with `resumedFromDisk == true` (or any run whose real id is known from disk) starts a turn, call `pool.setRealSession(rs.providerSessionID, rs.realProviderSessionID)` before spawning, so `claude_adapter` picks `--resume <realId>` (it already prefers the real id over the synthetic — `claude_adapter.go:189`).
- Ensure the Claude turn env (`CLAUDE_CONFIG_DIR`/`HOME`) targets the **active** account home (already the case via the adapter's `env`; the relocation in §6.3 put the file under that home).

### 6.5 — Codex resume turn (CLI path, per §3 decision)
Add a CLI-backed resume turn for runs with `resumedFromDisk == true` (or any chat run whose Codex thread must be resumed). Model it on `claude_process.go`:

New file `apps/local-runner/internal/runner/codex_resume_process.go`:
- Spawn `codex exec resume <rolloutSessionID> --all -c sandbox_mode="<derived>" -c approval_policy="<derived>" "<prompt>"`.
- Set `cmd.Env = append(os.Environ(), "CODEX_HOME="+activeAccountHome)`.
- Set `cmd.Dir = rs.workspaceCwd`.
- Map stdout to the same `ProviderEvent`s the app-server path emits (reuse `mapCodex*` where possible, or parse `codex exec`'s `--json` output if available; otherwise emit a single `EventMessageCompleted` + `EventTurnCompleted` from the final assistant message).
- Derive sandbox/approval from `codexYoloDerive(yolo)` (reuse existing).

Wire it: in `runTurn` / the registry adapter selection, when `rs.resumedFromDisk` and `rs.providerKey == codex`, route the turn through the CLI resume process instead of the app-server adapter. Fresh runs are unchanged.

> Note: `codex exec --json` / streaming output format should be confirmed from `codex exec --help` on the pinned version during implementation; if structured streaming isn't available, capture the final stdout block as the assistant message. This is an output-parsing detail, not a design fork.

---

## 7. Phase D — Desktop (unified history + greyed-out fallback)

File: `apps/desktop-flowpilot/src/components/Navigator.tsx` (history list), and the resume call site (`store.ts` / `RunStatus.tsx`).

- **D-7** Keep the history list **unified** — no account UI (D-2). Do not add badges/filters.
- **D-8** When the user clicks a history item, call resume. On a resume error with code in `{session_unavailable, account_not_signed_in, resume_unsupported}`, render that item **greyed-out / disabled** with the server's message as a tooltip (D-3). Do not navigate, do not crash.
- **D-9** A successful resume opens the run and allows a new turn exactly like an in-session run.

No new response fields are required for the unified list; `provider_account_id` stays server-side.

---

## 8. Phase E — Compatibility canaries in `scripts/quicktest.ps1` (item 1)

Purpose: catch a future Codex/Claude update that breaks session-file portability or the resume CLI surface — the same contract CA-098 relies on. These are **zero-cost** (no API/tokens). Add a new section after the existing flag checks.

```powershell
# ---- 6. Session portability contract (CA-098) --------------------------------
HDR "Session portability"

# 6a. Codex: `exec resume` CLI surface still exists (the resume transport).
$codexExecHelp = (& codex exec resume --help 2>&1) -join "`n"
if ($codexExecHelp -match "SESSION_ID" -and $codexExecHelp -match "PROMPT") {
    OK "codex exec resume <SESSION_ID> [PROMPT] present"
} else {
    FAIL "codex exec resume surface changed -- cross-account/restart resume will break"
}

# 6b. Codex: rollout session-meta is still account-agnostic (no account binding).
$codexHome = if ($env:CODEX_HOME) { $env:CODEX_HOME } else { Join-Path $env:USERPROFILE ".codex" }
$newest = Get-ChildItem (Join-Path $codexHome "sessions") -Recurse -File -Filter "rollout-*.jsonl" -ErrorAction SilentlyContinue |
          Sort-Object LastWriteTime -Descending | Select-Object -First 1
if ($newest) {
    try {
        $meta = (Get-Content $newest.FullName -TotalCount 1) | ConvertFrom-Json
        $p = if ($meta.payload) { $meta.payload } else { $meta }
        $hasId = [bool]$p.id
        $accountish = $p.PSObject.Properties.Name | Where-Object { $_ -match 'account|auth|user|token|email|org' }
        if ($hasId -and -not $accountish) {
            OK "Codex rollout meta is account-agnostic (id present, no account fields)"
        } else {
            FAIL "Codex rollout meta changed -- account binding ($($accountish -join ',')) or missing id breaks portability"
        }
    } catch { WARN "Could not parse newest Codex rollout meta: $($newest.Name)" }
} else {
    WARN "No Codex rollouts found under $codexHome -- skipping rollout-schema canary"
}

# 6c. Claude: --resume flag still exists (already covered by $CLAUDE_FLAGS list above)
#     and the projects/<hash>/<id>.jsonl session store still exists.
$claudeProjects = Join-Path $env:USERPROFILE ".claude\projects"
if (Test-Path $claudeProjects) {
    OK "Claude session store present (~/.claude/projects)"
} else {
    WARN "~/.claude/projects not found -- Claude session layout may have changed"
}
```

Also (optional, gated behind `-not $SkipProbe`): a full relocate+resume probe between two `CODEX_HOME`s, mirroring CA-098. Keep it OFF by default because it needs a logged-in second account and spends a turn; document it in CA-098 as the manual deep check.

**Update Task-059's compat config note:** the new canaries belong to the same "tested contract" idea, so they live in `quicktest.ps1` and run from the Check Version page's existing flow. No new config keys.

---

## 9. Tests

### Go (runner)
- `local_file_session_store_test.go`: `provider_account_id` + re-pointed `provider_session_id` survive restart (last-wins).
- `interactive_*_test.go`:
  - `resumeRun` reconstructs a chat run from the store after a simulated service recreation (new `InteractiveService` over the same store dir) and returns a valid `RunHandle` with `StepID = "chat-<id>"`.
  - `resumeRun` for a `workflow` run returns `resume_unsupported` (D-4).
  - `resumeRun` for a run whose account differs and whose target account lacks auth returns `account_not_signed_in`.
- `session_file_locator_test.go`: `LocateSessionFile` finds a rollout by id-suffix glob; `RelocateSessionFile` copies into the target home without overwriting and creates dirs.

### Manual (uses real accounts; see CA-098 checklist)
- Start a chat under account A. Restart the app. Open it from history → it opens and a follow-up turn works (same account).
- Switch active account to B (signed in). Open the same chat → it relocates + resumes under B; the run keeps its `run_id` and history.
- Open a chat whose account is signed out → history item is greyed-out with the reason; no crash.

---

## 10. Acceptance criteria

- After restart, clicking a chat run in history opens it and a new turn succeeds (no `run_not_found` for chat runs).
- A chat started under account A can be continued under a signed-in account B; `run_id` is preserved; `sessions.ndjson` shows the re-pointed `provider_account_id` / `provider_session_id`.
- Unopenable runs are greyed-out with a reason; never a crash; never history-injection.
- History list is unified (no account UI).
- `scripts/quicktest.ps1` gains the session-portability canaries and they pass on the pinned Codex/Claude versions.
- `go build ./...` clean; `go test ./internal/runner/...` green; desktop TypeScript compiles.

---

## 11. Deferred (explicitly NOT in this guide)

- **Cross-PC** (Google Drive sync of `sessions.ndjson` + provider files) → [Task-069](../../08-Task/todo/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md). The persistence + relocation primitives built here are the foundation; cross-PC adds upload/download + cwd-hash remap + `run_id` collision handling.
- **Workflow run resume** (re-fetch step list from the catalog by `workflow_id`) → Phase 2 (Task-067 T-6).
- **Codex multi-turn continuity via app-server** (`thread/resume`) — only if a future app-server exposes it; the CLI path here makes it unnecessary for the MVP.
```
