# CA-760 — ask_user question TTL 10m → 30m

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: BUG-288
change_type: hotfix
summary: Default ask_user questionTTL raised from 10m to 30m so a live CA-758 retest card is still answerable; approvalTTL stays 10m
# --->8---

## Why

Operator left the implement `TestDivide` question up during CA-758 retest (`run-208530`). After 10 minutes `AnswerQuestion` returned 409 `question_expired` and the TUI retried into a spam of the same error. TTL expiry itself is the BUG-288 contract (late answers must not stamp ghost RUNNING). The 10m default is too tight for a live harness walk.

## Change

- `newInteractiveService`: `questionTTL` `10 * time.Minute` → `30 * time.Minute`. `approvalTTL` unchanged at 10m.
- Comments on `AskQuestionCtx` / `askUserCtxBridge` updated to 30m so they match the constructor.

## Not changed

- Expiry still 409 `question_expired`. MCP disconnect still expires the card (CA-743). TUI leftover picker after expiry is a separate UX gap — not this change.
- Tests that override `questionTTL` (50ms / 1s / 5s / 30s) untouched.

## Tests

- New `question_ttl_default_test.go`: default constructor is 30m question / 10m approval.
- Related old expiry tests (`TestExpiredQuestionNotReplayedOnReconnect`, `TestWorkflowDrivenQuestionExpires`) still green — they override TTL.
