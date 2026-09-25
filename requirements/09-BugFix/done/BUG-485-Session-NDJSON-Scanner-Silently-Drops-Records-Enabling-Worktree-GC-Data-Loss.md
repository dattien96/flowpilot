# BUG-485 — sessions.ndjson scanner silently drops records → boot GC prunes live worktree bindings (+ all recent session history)

- Status: `done`
- Severity: **critical** — live-verified data loss on the real bed
- Found: 2026-09-25, Leg G live drill for BUG-473 (runner `instance_9a1af5b80fa7add2df7989b1e6b94345`, bed `/Users/tiendat/fp-beds/full`)
- CA: `change-audit/CA-984-bug485-session-ndjson-partial-read-data-loss.md`

## Live evidence

Bed `sessions.ndjson`: 2254 lines, **567 lines >64KB**, max line 126,488 bytes at line 1457.

Boot GC on the live runner (build `4f22f4698ad06444`) pruned all four worktree
dirs including two with **live `worktree_state=active` bindings**:

```
[worktree-gc] pruned orphan worktree owner=cht_67b99e9bd6fe  ← real binding, state=active
[worktree-gc] pruned orphan worktree owner=cht_fakegc473     ← worktree_state="" → should be gc_deferred
```

## Root cause (two defects, same family)

**D1 — `bufio.Scanner` 64KB token cap + unchecked `sc.Err()`.**
`local_file_session_store.go` reads durable NDJSON files with
`bufio.NewScanner(f)` in five places:

| Line | Function | File | Failure mode |
|------|----------|------|--------------|
| ~244 | `loadFromDisk` | `sessions.ndjson` | Scan aborts at first >64KB line → all records after it never reach the session map |
| ~284 | `loadQuestionsFromDisk` | `questions.ndjson` | same silent truncation |
| ~319 | `loadApprovalsFromDisk` | `approvals.ndjson` | same |
| ~386 | delete-rewrite | `sessions.ndjson` | Scan aborts mid-file → `kept` misses every record after the fat line → **atomic rename destroys them permanently** |
| ~613 | provider-event load | per-run event log | same silent truncation |

`sc.Err()` is never checked anywhere, so the truncation is invisible — no
log, no error, the store "loads" partially and reports healthy.

**D2 — `loadFromDisk` drops chat-run records.**
`rec.ProjectID == ""` is skipped, but normal-chat runs legitimately have no
project (`project_id: ""` on every chat run). Their session records —
including `worktree_*` binding fields — are silently discarded at boot even
when the scanner reaches them.

## Blast radius

1. **Worktree GC** (`worktreeGCVerdictFor`): dropped session rows →
   `GetProviderSession` not-found + `ListProviderSessionsByChat` empty →
   `gcOrphan` → live worktree dir + sidecars deleted on every boot. The
   BUG-473 fail-closed contract never engages because no reader *errors* —
   the data is simply absent.
2. **Session restore/resume**: ~800 newest records invisible → resume,
   leg discovery, chat history reconstruction blind for recent runs.
3. **DeleteProviderSession rewrite**: a single delete call can destroy all
   records after the first fat line — data loss on a *write* path.
4. **Questions/approvals replay**: same truncation → pending questions/
   approvals silently forgotten.

## Fix contract

1. Replace every durable-file `bufio.Scanner` read in
   `local_file_session_store.go` with a shared line reader that has **no
   64KB cap** (`bufio.Reader.ReadBytes('\n')`, matching
   `dispatch_store_local.go`'s existing pattern) and **surfaces truncation
   as an error** — fail-closed, per `additive-tests-only` + durability
   rules.
2. `localFileSessionStore` records a load error when any durable scan
   truncates; `ListProviderSessionsByChat` / `GetProviderSession` /
   `ListAllProviderSessions` return that error so `worktreeGCVerdictFor`
   gets `gcUnknown` → `gc_deferred` instead of `gcOrphan`.
3. `loadFromDisk` must not drop records on `ProjectID == ""` — chat runs
   have no project by design. Keep the `RunID == ""` skip.
4. The delete-rewrite path must error out (not rewrite) when the source
   scan is incomplete — never let a partial read destroy records.

## Required tests (RED first)

- `TestBUG485_SessionLoadSurvivesOversizedLine`: sessions.ndjson with a
  >64KB record before a worktree-bound record → boot load →
  `GetProviderSession` returns the bound row; sweep verdict is `gcBound`.
- `TestBUG485_TruncatedSessionLoadDefersGC`: inject a line that can't be
  read (or simulate a read error past the cap pre-fix) → GC verdict
  `gcUnknown` → `gc_deferred`, worktree dir survives.
- `TestBUG485_ChatRunRecordWithEmptyProjectLoads`: chat-run record
  (`project_id:""`, `worktree_state=active`) loads and feeds the GC
  verdict.
- `TestBUG485_DeleteRewritePreservesOversizedLines`: delete one run's
  record from a file containing a >64KB line → all other records survive.
- `TestBUG485_QuestionsAndApprovalsSurviveOversizedLine`: same fat-line
  file for questions/approvals → pending records replay after restart.
- Regression: existing BUG-473 GC tests + boot sweep tests stay green.

## Implementation plan

- P-1: RED tests above (assertion-level reproduction, no prod changes).
- P-2: shared helper `scanNDJSONLines(path) ([][]byte, error)` (or
  `readNDJSONFile`) using `bufio.Reader.ReadBytes('\n')`; swap all five
  call sites.
- P-3: `localFileSessionStore.loadErr` recorded on truncation; exposed via
  the reader methods the GC consults.
- P-4: remove the `ProjectID == ""` drop in `loadFromDisk`.
- P-5: rewrite path fails closed on scan error (no rename).
- P-6: live re-verification on the bed: recreate worktree binding + fat
  file → restart → binding survives, `gc_deferred`/bound, recent sessions
  restored.

## Definition of Done

- [ ] All RED tests green after fix
- [ ] `internal/runner` suite green (baseline env failures aside)
- [ ] Live: worktree binding survives restart on the real bed with the
      567-fat-line file; `gc_deferred` or `gcBound`, never silent prune
- [ ] Live: recent chat sessions restored post-restart
- [ ] CA-NNN entry + commit `[Fix][runner] BUG-485: ...`
