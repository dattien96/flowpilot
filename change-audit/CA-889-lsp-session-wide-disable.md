# CA-889 — CP-63 M-4: LSP crash-disable was manager-scoped, not session-wide

# ---8<--- flowpilot:change-ledger
feature_key: lsp-runtime
source_doc_id: CP-63
change_type: bugfix
summary: Crash-budget disable now session-wide in ServerSet (disabled map + getOrStart guard + Disabled accessor); additive live-replay test green; verified live on gate-sandbox bed that a post-disable write turn no longer respawns gopls
# --->8---

## Why

CP-63 M-4 live verification (2026-09-17, runner-owned gopls,
`/tmp/lsp-m4-bed`, report `/tmp/cp63-m4-report.md`) confirmed the lifecycle
`lsp.start → lsp.restart (attempt 1) → lsp.disabled (crash budget spent)`,
but the NEXT file-writing turn logged a brand-new `lsp.start` with a fresh
crash budget. Root cause (production files, read-only at the time):

- `internal/lsp/server_manager.go` — `disabled bool` is a per-`ServerManager`
  field; nothing session-wide was recorded when `healthLoop` spent the budget.
- `internal/lsp/runner_hook.go` `getOrStart` — a non-running manager was
  deleted from `ServerSet.servers` and a fresh `&ServerManager{}` (restarts=0,
  disabled=false) was spawned on the next check.

Consequence: a crashy language server would be re-spawned (and crash again)
on every later turn — the R-1 mitigation ("crash twice → disabled for the
session") did not hold across turns.

## Change

- `internal/lsp/server_manager.go`: additive `Disabled()` accessor exposing
  the existing private `disabled` flag.
- `internal/lsp/runner_hook.go`:
  - `ServerSet` gains `disabled map[serverKey]bool` (session-scoped) and
    `disabledNotified map[serverKey]bool` (warn-once);
  - `getOrStart` records `disabled[key]` when the dead manager was
    crash-disabled, and returns an error (skip, degrade silently) instead of
    spawning while that flag is set, logging once:
    `[lsp] server for <root>/<platform> disabled for this session (crash
    budget spent earlier); build/test validation remains the backstop`.
- New additive test `internal/lsp/runner_hook_session_disable_test.go`
  (RED before the fix — post-disable check respawned a server; GREEN after)
  with its own crash-after-init helper process; no existing test edited.

## Impact

- `npx gitnexus impact getOrStart --repo flowpilot` → LOW risk, single direct
  caller `CheckFiles` (d=1), 0 processes affected. `Disabled()` is additive.
- Degradation semantics unchanged: `CheckFiles` still returns "" on every
  skip path (missing binary, cooldown, now session-disabled), so the gate
  hook keeps falling back to the build/test oracle.

## Verification (this machine, macOS, 2026-09-17)

- `go test ./internal/lsp/... -count=1` → `ok ... 48.280s` (full suite incl.
  the new test); runner LSP wiring scope
  `TestRunnerInjectsLSP|TestRunnerSkipsLSP|TestHandleLSPStatus|TestLSPDiagnosticsSource|TestPostWriteHook`
  → `ok`; `go build ./...` + `go vet ./internal/lsp/` clean.
- Live re-verification (fresh runner `/tmp/fp-verify-runner-lspfix` :18768,
  bed `/tmp/lsp-m4-bed2`, run-861497, grok/grok-4.5):
  `21:39:51 lsp.start` → SIGKILL → `21:40:00 lsp.restart (attempt 1)` →
  SIGKILL → `21:40:08 lsp.disabled (crash budget spent)` → post-crash write
  turn turn-861970 COMPLETED at `21:40:54` logging
  `server for /tmp/lsp-m4-bed2/golang disabled for this session (crash budget
  spent earlier); build/test validation remains the backstop` and **no** new
  `lsp.start`/`lsp.restart`; `bed.go` gained `Div` via build/test validation.
  Log: `/tmp/fp-r-lspfix.log`.

## Not done / residual risks

- The session-disable flag lives in the process (as designed): a runner
  restart legitimately starts a fresh session.
- The Android Gradle build-fallback validator (Task-361) remains the
  Android-specific backstop; on Go projects the go-test oracle carries the
  safety burden after disable (observed live, no reprompt).
