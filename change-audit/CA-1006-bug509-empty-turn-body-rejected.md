# CA-1006 — BUG-509: empty/unknown turn body rejected instead of dispatching a no-op turn

## Context

Live operator error during the rerun: a turn request carrying `text`
instead of `prompt` was silently accepted — the unknown field dropped, an
empty prompt dispatched, devin no-op'd, and the post-turn gate spawned an
owner-debate loop off a phantom turn. Silent acceptance made the footgun
indistinguishable from a real user turn.

## Changes

- Turn handler (`interactive_handlers.go`): a body whose meaningful fields
  are all empty — no `prompt`, no attachments, no `flowRef`, no `subMode`,
  no `sourceDocId`, no `changeType` — is rejected `400 empty_turn_body`
  before dispatch. Attachment-only and flow-launch requests remain valid;
  only the bare no-content body is refused.
- Regression test `bug509_empty_turn_body_test.go`: `{"text":"…"}`-style
  bodies get a typed 400; attachment-only and flowRef-only bodies still
  pass.

## Verification

`go test -count=1 -run TestBug509 ./internal/runner/` — green.
