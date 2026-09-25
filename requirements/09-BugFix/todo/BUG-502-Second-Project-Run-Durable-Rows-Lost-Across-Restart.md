# BUG-502 — Second-project chat run durable rows lost across runner restart

## Status
Open — live-found 2026-09-25 on build b501. Symptom confirmed; write-path
exonerated on current build; exact loss mechanism unproven. Needs an
instrumented repro before a fix is landed (per safe-fix contract: no blind
fix).

## Live-found during
CP-84 mux matrix + B-59 durability drill on bed `/Users/tiendat/fp-beds/full`.

- `run-260777` (project `db51ec26-...` = Gate-sandbox, second registered
  project): completed a real grok turn (`PONG-B` + transcript persisted to
  `run-260777-turns.ndjson`), then a write-intent turn parked it
  `waiting_approval` with gate card + approval `appr-260887`.
- `run-260889` = devin leg of the same chat (`cht_12594b9ce216`).
- After `kill -9` + restart (boot 19:36:34):
  - `GET /client/workflow-runs/run-260777` → `run_not_found`
  - chat timeline for `cht_12594b9ce216` → `chat_not_found`
  - pending approval `appr-260887` unrecoverable.
- Durable file audit: `sessions.ndjson` / `approvals.ndjson` /
  `questions.ndjson` contain **zero** rows for run-260777/260889; the shared
  session file holds only `project_id=957928cc` (workspace) rows — 377 rows.
  The per-run turns file for run-260777 still exists, so the run was never
  deleted through `deleteChatSession` (which would have removed it too).
- Control: `run-260775` (workspace project, same shape, seconds apart) —
  4 session rows present, rehydrates as `cancelled` after restart.

## Evidence collected

| Check | Result |
|---|---|
| `persistProviderSession`/`persistApproval` store plumbing | Unconditional — single `persistenceStore()` = `localFileSessionStore`, no project gate |
| In-process reproducer (`bug502_cross_project_session_persist_test.go`) | **PASSES** — second-project run writes session rows on current build |
| Post-restart live recheck (`run-266599`, project db51ec26) | 3 rows written, survives a graceful restart, rehydrates `cancelled` |
| Gate diff log 19:23:32 | `sessions.ndjson Status:M` — file was modified during the affected window |
| `DeleteProviderSession` rewrite | Drops only target runID + malformed lines; aborts on read error — cannot explain well-formed rows vanishing |
| `collectDeleteRunTree` / boot recovery | Subtree-only walk; recovery logs "reconstruct … run not found" and leaves state — no delete |
| Concurrent writer | A second runner process (`flowpilot-runner-live`, build `e85776b3`, port 19500) shares cwd → same `sessions.ndjson`. It was idle all day (log ends 10:17), so it is a hazard but not a proven actor |

## Candidate mechanisms (unproven)

1. **Cross-process file rewrite**: an older-build runner sharing the dataDir
   calls `DeleteProviderSession`, whose read+rewrite drops lines that
   unmarshal-fail under ITS schema (schema drift between builds). Requires
   the dropped lines to be malformed under the old schema — not verified.
2. **Torn-tail + rewrite ordering at kill -9**: a partial append at the tail
   sets `sessionsLoadErr`; a later rewrite path that ignores the torn tail
   could drop adjacent rows. Requires proving which path wrote last.
3. Callsites for the affected runs never fired because the turns took a
   non-`startTurn` path — contradicted by the persisted prompt/transcript
   lines (`AppendTurnLog` at startTurn ~:10075 runs after the persist at
   ~:10069 in the same straight-line block).

## Next step (do not skip)

Instrument before fixing:

- `DeleteProviderSession`: when dropping a non-target line (malformed),
  `log.Printf` the embedded `run_id`/line hash so a post-hoc audit can name
  the victim and the drop reason.
- `loadFromDisk`: log per-run_id when `sessionsLoadErr` is set or a row is
  aged out — so a post-restart diff between file and index is observable.
- Re-run the exact live drill (second-project grok turn → gate park →
  `kill -9` mid-activity → restart) with the file snapshotted before kill.

Then RED → fix → re-drill per the standard workflow.

## Regression guard

`internal/runner/bug502_cross_project_session_persist_test.go`
(`TestBug502SessionRowPersistsForSecondProjectRun`) — asserts a chat run on a
non-workspace project ID leaves a durable session row. Currently **passes**:
it locks in the verified-good write path so a future refactor cannot
reintroduce a project-scoped persist gap silently.
