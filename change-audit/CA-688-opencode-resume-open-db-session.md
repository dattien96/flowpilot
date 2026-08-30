# CA-688 — Open locally reopenable: resume precheck treats the opencode DB-backed session as available

## Problem

Operator hit this on the CP-57 test guide's smoke step: `/open` of an old
opencode chat (run-329695) failed with `Open failed (runner session_unavailable):
session data not found on this machine`. This was the gap documented in CP-57
§10.2: `ensureResumeReady` demanded a per-session FILE via `LocateSessionFile`,
but opencode 1.18.x stores sessions in the shared
`~/.local/share/opencode/opencode.db` — and ACP `session/load` works from any
process on this machine (BUG-329 live probe), so the file check is the wrong
availability signal for opencode.

## Fix (opencode-only; grok/codex/claude/gemini unchanged)

`ensureResumeReady` gained an early opencode branch, before the account-home /
`locateSessionAcrossProviderAccounts` / `LocateSessionFile` flow:

- A real `ses_*` id (`isOpencodeRealSessionID`) is enough to resume — the
  transcript replays via the existing `seedOpencodeTranscriptFromDisk` and the
  next turn continues the same session through ACP `session/load`.
- Synthetic `thread-*` ids still cannot resume (typed `session_unavailable`).
- The auth check (`HasLocalAuthAtPath`, CA-659 paths) still runs when the
  account home resolves — a signed-out active account still surfaces
  `account_not_signed_in`.
- Drive file-copy `/sync` → `/restore` remains typed-unsupported by design.

## Follow-up (same CA): /open replayed prompts without answers

After the reopen fix, the operator saw the transcript replay only user prompts.
Root cause: the durable `transcript_turn` gate (prompt/assistant persistence for
providers without their own transcript file) was `Gemini || Grok` only —
opencode turns logged prompt lines + the session id but never the assistant
text. Gate extended to `ProviderKeyOpencode`
(`interactive_service.go`, one line + rationale comment).

Applies to turns from the fix onward; turns logged before the fix have no
durable assistant text (recovery from opencode.db would be the deferred
transcript extractor, Task-303 row 20).

## Tests

- `ca688_opencode_resume_open_test.go` — `resumeRun` succeeds for an opencode
  chat with a real `ses_*` id and NO provider accounts configured and NO
  session files on disk (the bypass must fire before home resolution);
  causal check verified by stashing the fix (baseline fails with
  `session_unavailable`, with the fix it passes).
- `ca688b_opencode_transcript_replay_test.go` — full integration: opencode turn
  on a real local-file store logs a `transcript_turn` with prompt AND assistant;
  `resumeRun` replays both sides.
- Resume/turn-log/seed/grok/gemini/opencode subsets green; `TestResumeFlowWithFeedbackAfterEscalate`
  remains the documented pre-existing baseline failure.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: Open locally reopenable — resume precheck accepts the opencode DB-backed session with a real ses id and no session files
# --->8---
