# CA-984 — BUG-485: sessions/questions/approvals NDJSON partial reads dropped records, enabling worktree-GC data loss

## What changed

`apps/local-runner/internal/runner/local_file_session_store.go`:

- New `readNDJSONLines(io.Reader, fn)` helper built on `bufio.Reader.ReadBytes('\n')`
  — no token cap — replacing `bufio.Scanner` (64 KiB default limit) at every
  durable-file read site: `loadFromDisk`, `loadQuestionsFromDisk`,
  `loadApprovalsFromDisk`, `DeleteProviderSession` rewrite, `ReadTurnLog`.
- `loadFromDisk` no longer requires `project_id != ""`: `run_id` is the minimum
  identity. Normal chat runs persist `project_id:""` by design.
- New fail-closed load tracking: `sessionsLoadErr`, `questionsLoadErr`,
  `approvalsLoadErr` (`atomic.Value`). Any incomplete read is stored; reader
  methods (`GetProviderSession`, `ListProviderSessionsByChat`,
  `ListAllProviderSessions`, `ListProviderSessionsByProject`,
  `ListQuestionsByRun`, `ListApprovalsByRun`) surface it as an explicit error so
  trust-sensitive consumers (worktree GC, resume) fail closed instead of
  acting on a truncated view. Stored errors are wrapped via `fmt.Errorf` so the
  atomic holds a single concrete type.
- Torn-tail detection (`fileEndsWithNewline` + `errTornTailRecord`): a final
  line that is unterminated AND unparseable is the signature of a crash
  mid-append; the store reports incomplete rather than silently dropping the
  record. Mid-file malformed lines keep the legacy skip behaviour.
- `DeleteProviderSession` aborts with an error on an incomplete read instead of
  atomically renaming a rewrite built from a partial view (which would have
  destroyed the unread suffix permanently).

## Why

Live incident on the `full` bed: `sessions.ndjson` held 2,254 lines with 567
lines over the 64 KiB `bufio.Scanner` token cap (max ~126 KiB — records embed
long transcripts/last_prompt). `Scan()` stopped at the first oversized line
and `Err()` was never checked, so every later record — including active
worktree bindings — was invisible at boot. `sweepOrphanedWorktrees` then
classified live bindings as orphans and pruned them. Two companion defects in
the same file: chat records with `project_id:""` were dropped entirely, and
`DeleteProviderSession` rewrote the file from the truncated read, permanently
deleting the unread suffix.

## How found

BUG-473 live drill: after seeding an orphan worktree + a corrupt-state record
and restarting the runner, a real active binding (`cht_67b99e9bd6fe`,
`worktree_state=active`) was pruned alongside the intended orphan. Inspection
showed the record's `project_id:""` and, separately, that hundreds of file
lines exceeded the scanner cap.

## Compatibility

- Provider-agnostic: local durable persistence only — no adapter, event
  stream, or session-pinning surface touched.
- Old files read identically; only previously-dropped records now load.
- Consumers that ignored reader errors see new errors only when a durable file
  genuinely could not be fully read — the correct fail-closed direction per
  the durability contract.

## Tests

`internal/runner/bug485_session_ndjson_scanner_test.go` (6 tests, all
reproduce-first red):

- oversized line no longer hides later bound session records;
- loaded store yields `gcBound`, never `gcOrphan`;
- chat records with empty `project_id` load;
- delete-rewrite preserves records after oversized lines;
- questions/approvals survive oversized lines;
- torn tail record fails closed to `gcUnknown`.

Full `./internal/...` suite run on this change: failures limited to the known
environment baseline (machine Supabase config leak, credential-dependent
provider/MCP tests, TempDir cleanup flakes) — no new regressions.
