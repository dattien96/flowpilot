# BUG-487 — transcript/session-file loaders silently truncate at scanner cap → resume replays incomplete context

- Status: `todo`
- Severity: **high** — silent context loss on resume; the model's restored
  transcript and any consumer of the scaffold-progress tail see a *shortened*
  history with no signal that data was dropped.
- Found: 2026-09-25, deep audit pass 2 (same defect family as BUG-485, on
  provider-transcript readers instead of session-store readers).

## Root cause

`bufio.Scanner` with a finite `scanner.Buffer` cap and **no `scanner.Err()`
check after the loop**. When any single line exceeds the cap (or the
underlying read fails mid-file), `Scan()` returns false and the loop exits
exactly as if EOF had been reached — the parsed prefix is returned as if it
were the complete file.

| Site | File | Cap | Caller impact |
|------|------|-----|---------------|
| `transcript_loader.go:24` `loadClaudeTranscriptEvents` | Claude `*.jsonl` | 4 MiB/line | resume replay drops tail turns |
| `transcript_loader.go:66` `loadCodexTranscriptEvents` | Codex rollout JSONL | 4 MiB/line | same |
| `grok_transcript_loader.go:52` `loadGrokTranscriptEvents` | Grok `chat_history.jsonl` | 4 MiB/line | same |
| `session_file_locator.go:574` `readCodexRolloutMeta` | Codex rollout line 1 | **default 64 KiB** | `session_meta` first line >64 KiB → `ok=false` → locator cannot validate candidate → wrong/absent session file resolution |
| `scaffold_progress.go:368` `readScaffoldProgressTail` | `.flowpilot/` progress log | 1 MiB/line | tail silently short |

Provider transcript lines routinely carry base64 attachments, tool outputs
and embedded file contents — a >4 MiB line is realistic on a large turn,
and Codex `session_meta` first lines can exceed 64 KiB.

## Blast radius

1. `interactive_resume.go:3292/4302` — resume transcript replay seeds
   `rs.events` from the parsed prefix only → the restored transcript shown
   to clients (and any context rebuilt from it) is missing every turn after
   the first oversized/unreadable line. **Nothing reports the loss.**
2. `session_file_locator.go:441/542` — meta read failure is indistinguishable
   from "no meta": wrong session-file selection on compare paths.
3. `scaffold_progress.go:252` — progress tail used by tooling/UI returns
   fewer events than the log actually holds.

## Fix contract

1. Change the three transcript loaders + `readCodexRolloutMeta` +
   `readScaffoldProgressTail` to surface read failures instead of swallowing
   them:
   - Transcript loaders: signature `func(string) []ProviderEvent` →
     `func(string) ([]ProviderEvent, error)`; return parsed prefix **and**
     the scan error (prefix is still usable context — the contract is "never
     *silently* lose", not "refuse partial data").
   - `readCodexRolloutMeta`: replace the scanner with
     `bufio.Reader.ReadBytes('\n')` for the single line (no cap); a read
     error and an unparseable/empty first line both still return `ok=false`
     (same outcome — file can't be validated), but the error must be
     distinguishable internally for logging.
   - `readScaffoldProgressTail`: `[]ScaffoldProgressEvent` →
     `([]ScaffoldProgressEvent, error)`; caller logs on error.
2. Update callers in `interactive_resume.go` and `handoff_context.go` to log
   a loud `[transcript-load]` warning with runID + path + error when a
   loader reports a partial read, then continue with the parsed prefix.
3. Keep the 4 MiB-cap *safety* motivation but remove the silent truncation:
   use the `readNDJSONLines`-style `bufio.Reader` pattern (no cap) from
   BUG-485 for transcript files, or a larger cap + `Err()` check — the
   observable contract is "error surfaces", not "unbounded memory".

## Required tests (RED first)

- `TestBUG487_ClaudeTransloadTruncatedLineReturnsError`: JSONL file with a
  >4 MiB JSON line followed by a valid line → loader returns the valid
  prefix **and** a non-nil error.
- `TestBUG487_CodexTransloadTruncatedLineReturnsError`: same for Codex loader.
- `TestBUG487_GrokTransloadTruncatedLineReturnsError`: same for Grok loader.
- `TestBUG487_RolloutMetaOversizedFirstLine`: rollout file whose first line
  is >64 KiB → meta parses successfully (no cap).
- `TestBUG487_ScaffoldTailReadErrorPropagates`: unreadable/oversized mid-line
  → error returned, not silently short tail.
- E2E: seeded transcript replay path test — run whose provider file
  truncates → resume completes but emits the `[transcript-load]` warning.

## Implementation plan

1. RED tests above (assertion: `err != nil` / meta parsed).
2. Change loader signatures, update the `loader func(string) []ProviderEvent`
   indirection in `interactive_resume.go` (switch cases assign the funcs —
   keep a small adapter or change the var type together).
3. Add the warning log at each call site; run `go test ./internal/runner/`.
4. CA entry + commit `[Fix][runner] ...`.

## Definition of Done

- No transcript/meta/progress reader can exit its scan loop on an error
  without that error reaching a caller that logs or propagates it.
- All new tests green; full `internal/runner` suite green; CA written.
