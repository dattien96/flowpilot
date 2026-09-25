# BUG-496 — chat_session Drive index scans silently truncate at 64KB → preserved/remote rows dropped on merge

## Status
todo — discovered in deep-review round 2 (missed scanner-cap site inside the same file BUG-483 hardened)

## Severity
Medium-high — silent remote data loss in the merge path

## Symptom

Two index scans in `chat_session_sync.go` use `bufio.Scanner` with the
default 64KiB token cap and never check `scanner.Err()`:

- `mergeChatSessionDriveIndex` (~:313) — scans the existing index bytes to
  collect `preserved` rows (malformed / identity-less rows that must be
  kept verbatim so other devices' data survives a merge).
- `parseChatSessionDriveIndex` (~:355) — scans the index into typed
  records for read paths.

A single index line longer than 64KiB (a corrupt remote upload, a
pathological `manifest_path`/metadata blob) trips `bufio.ErrTooLong` —
`Scan()` returns false, iteration stops silently, and **every row after
the oversized line is dropped**:

- in the merge path, dropped `preserved`/typed rows are missing from the
  re-uploaded index → rows another device wrote are clobbered.
- in the parse path, callers see a partial index — a record that exists
  remotely appears absent.

The preservation contract (`preserved` tail exists specifically so remote
rows are never silently dropped) is violated by the scanner itself.

## Root cause

Default `bufio.Scanner` cap on an in-memory reader, `Err()` unchecked —
the same primitive BUG-487 fixed on file-backed loaders; these two sites
were missed because they scan `strings.Reader`/`[]byte` rather than files.

## Fix (implemented)

Both functions switch to `readNDJSONLines` (bufio.Reader.ReadBytes —
uncapped, error-propagating). Inputs are in-memory buffers, so the only
error source disappears with the cap: signatures are unchanged and callers
need no mechanical updates.

## Tests

- `bug496_drive_index_scan_cap_test.go`
  - RED: existing index containing a >64KB malformed line followed by a
    valid remote row → `mergeChatSessionDriveIndex` output preserves BOTH
    the oversized line and the trailing row verbatim.
  - `parseChatSessionDriveIndex` over the same input returns the trailing
    valid record (not silently absent).
  - Healthy short-line input unchanged.
