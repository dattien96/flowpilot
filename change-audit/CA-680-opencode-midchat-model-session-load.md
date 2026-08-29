# CA-680 — OpenCode mid-chat model switch keeps the live acp process and session

## Problem

run-307050: first turn on `opencode-go/muse-spark-1.2-contributor` streamed fine;
switching to `opencode-go/deepseek-v4-flash` mid-chat failed all retries with
`turn failed: opencode session/load returned no sessionId`.

Live probe (opencode 1.18.18): `session/load` of a `ses_*` created in `opencode acp`
process A, issued on a fresh process B, returns RPC-OK with a config-only result and
NO sessionId — while `session/prompt` on the loaded id still completes (`end_turn`;
sessions persist in the shared `opencode.db`). `ensureOpencodeProcess` keyed by
scope+model+variant+auto (Grok copy), so every mid-chat model change spawned a fresh
process and orphaned the live session. `turnResumeProviderSessionID` also lacked the
`lastOpencodeTurnSessionID` fallback that Grok has (run-92955 twin).

## Fix (OpenCode-only; Claude/Codex/Grok untouched)

- `opencode_process.go` `ensureOpencodeProcess`: exact-key hit still wins; on a miss
  reuse any live same-scope handle. Model/effort/YOLO changes no longer respawn;
  account/scope switch still closes every other scope. `opencodeProcessKey` unchanged.
- `opencode_adapter.go` `ensureSession`: `session/load` ok-without-sessionId adopts the
  requested `ses_*` id (never a silent `session/new`); real RPC errors still fail.
- `interactive_service.go` `turnResumeProviderSessionID`: opencode falls back to
  `lastOpencodeTurnSessionID` (Grok parity); `interactive_resume.go` seeds it on restore.

## Tests

- `bug329_opencode_midchat_model_session_load_test.go` (10 cases): same-scope reuse on
  model/variant/YOLO change, exact-key priority, scope-switch close without spawn,
  SendTurn over config-only load (adopt + persist + complete, no session/new), load RPC
  error still fails, missing-id without resume still errors, `thread-*` first turn still
  `session/new`, resume-fallback + Grok/Codex parity guards.
- Old suite: `TestOpencode*`/`TestEnsureOpencode*`/`TestGrok*` green;
  `TestResumeFlowWithFeedbackAfterEscalate` remains a pre-existing baseline failure
  (verified identical with CA-679/680 changes stashed).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: OpenCode mid-chat model switch reuses the live acp process and adopts the resumed ses id when session/load omits sessionId
# --->8---
