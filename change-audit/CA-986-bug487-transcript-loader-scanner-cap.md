# CA-986 — BUG-487: transcript/session-file loaders no longer truncate at the scanner cap

## What changed

`apps/local-runner/internal/runner/`:

- `transcript_loader.go`: `loadClaudeTranscriptEvents` and
  `loadCodexTranscriptEvents` now return `([]ProviderEvent, error)` and read
  via `readNDJSONLines` (BUG-485 helper, uncapped `bufio.Reader`) instead of
  a 4 MiB-capped `bufio.Scanner` whose `Err()` was never checked.
- `grok_transcript_loader.go`: same signature change; `bufio` import dropped.
- `session_file_locator.go`: `readCodexRolloutMeta` reads uncapped.
- `scaffold_progress.go`: `readScaffoldProgressTail` returns
  `([]ScaffoldProgressEvent, error)`; the loader logs the partial read.
- `interactive_resume.go`: transcript loader type updated; a partial read
  logs the error and continues replay with the parsed prefix (a torn tail
  degrades visibly instead of silently).

## Why

Every one of these readers hit `bufio.Scanner`'s 64 KiB default / 4 MiB
custom cap and discarded `scanner.Err()`. A single oversized NDJSON line
(provider frames can carry embedded artifacts) silently dropped the line
*and every line after it* — resume replayed a truncated transcript and
reconstructed an incomplete context with no signal. Codex rollout meta and
scaffold progress had the same class of silent tail-loss.

## Contract

- Missing/unopenable files still return `(nil, nil)` — absence is not an
  error.
- A readable oversized line is fully consumed and parsed.
- A genuine mid-file read error returns the parsed prefix plus the error;
  callers log and degrade, never silently claim completeness.

## Verification

- `bug487_transcript_truncation_test.go`: multi-MiB line followed by a valid
  line parses fully; injected read fault returns the error; oversized Codex
  rollout meta and scaffold progress tails parse; missing-file behavior
  unchanged.
- `go test -count=1 ./internal/runner/` green.
