# CA-995 — BUG-496: Drive index scans use the uncapped reader

## What changed

`apps/local-runner/internal/runner/chat_session_sync.go`:

- `mergeChatSessionDriveIndex` and `parseChatSessionDriveIndex` switched
  from `bufio.Scanner` (default 64KiB cap, `Err()` unchecked) to
  `readNDJSONLines` — a single oversized remote-written line no longer
  truncates the scan and drops `preserved` rows / trailing records.
- Signatures unchanged (in-memory inputs); all callers untouched.

## Invariant

The BUG-483 preservation contract is only truthful when the scan sees the
whole file — an oversized line must never amputate the preserved tail.

## Tests

`bug496_drive_index_scan_cap_test.go`: >200KiB malformed line + valid
trailing row → merge output contains both verbatim; parse returns the
trailing record.
