# CA-967 — Devin ACP process-exit watcher (F-1 dead-handle fix)

## Change

`ensureDevinProcessSegmented` (`devin_process.go`) now spawns a goroutine
`_ = cmd.Wait(); dispatcher.fail("devin acp process exited")` right after
`dispatcher.start(stdout)`. Process exit is the authoritative liveness
signal; stdout EOF is a secondary path only.

## Why

Live test of Task-438/439 (Task-438-439-Test-Steps §5) exposed F-1: killing
the `devin acp` parent left MCP grandchildren inheriting the stdout pipe, so
the dispatcher read loop saw no EOF for ~142s. During that window the dead
handle still reported warm (`closed=false`): no `provider_status` emitted,
the next turn reused the handle, wrote `session/load` into a pipe with no
reader, and failed late with `devin acp stream closed: EOF`.

`cmd.Wait()` returns on process exit regardless of pipe inheritance and also
closes the parent-side pipes, unblocking the read loop. `dispatcher.fail` is
idempotent, so racing with readLoop EOF or `h.close()` is safe (first wins).

## Test

- `devin_process_exit_test.go::TestEnsureDevinProcessExitMarksClosedWithoutStdoutEOF`
  — additive red-first test: fake-bin `sh -c` spawns `sleep 60 &` grandchild
  holding stdout before the handshake, parent killed → asserts `isClosed()`
  within 3s. Red before fix (3.07s timeout), green after (0.08s).
- Focused devin suite (`-run 'Devin|devin'`): all green.
- Live re-verify on real account (binary `7fc921a9+dirty`, runner :47788):
  `taskkill` on the prewarmed ACP child → next turn emitted
  `provider_status connecting` (09:19:44.295) → `ready` (09:19:53.807,
  ~9.5s cold handshake) → `session/load` resumed `brook-monkey` → answered.
  Before fix the same scenario cost ~142s hang + EOF failure.

## Risk / parity

- Runner-internal only; no event/session/contract shape change. Claude,
  Codex, Grok, Gemini, Opencode paths untouched (separate adapters).
- One extra idle goroutine per live devin process — negligible.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-439
change_type: bugfix
# --->8---
