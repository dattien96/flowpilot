# CA-324: hub.notify Deferred Reinvoke Now Retries With Its Own Prompt

## Scope

Fixes BUG-284: `hub.notify`'s "done" is observed mid-turn (via `SubmitFlowControl`), so the reinvoke defers on essentially every dispatch. The turn-completion retry unconditionally fired the generic synthesis reinvoke instead of the hub.notify node's own write-contract prompt, silently dropping the notify dispatch — no Telegram message was ever actually sent, with no error surfaced.

## Changes

- `apps/local-runner/internal/runner/interactive_service.go`:
  - added `interactiveRun.pendingHubReinvokePrompt string`.
  - `maybeAutoReinvokeHubWithPrompt`'s `turnInFlight` defer branch now stashes the caller's exact prompt into this field alongside the existing `pendingHubReinvoke` boolean.
  - the turn-completion retry site (`runTurn`'s completion path) now checks `pendingHubReinvokePrompt` first: non-empty → `maybeAutoReinvokeHubWithPrompt(rs.id, prompt)`; empty → the pre-existing `maybeAutoReinvokeHub(rs.id)` unchanged.
- New test `TestHubNotifyDeferredReinvokeRetriesWithStashedPromptNotGenericOne` (`flow_hub_notify_test.go`): fake adapter calls `SubmitFlowControl(done)` mid-turn (reproducing the exact live defer condition), asserts the retried turn's prompt is the hub.notify write-contract, not the generic synthesis text.

## Verification

- `go build ./...` — passed. `go vet ./internal/runner` — no issues.
- `go test ./internal/runner -run 'HubNotify'` — all passed.
- Broad regression sweep (`Flow|Review|Orchestrat|Cohort|Hub|Reinvoke|Telegram|Audit|Validate|Behavior`) — no new failures against the pre-change baseline.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via direct inspection of the diagnostic log (`run-8491`) and the named code paths.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-284
change_type: bugfix
summary: stash the hub.notify node's own write-contract prompt when its reinvoke defers mid-turn, and retry with that exact prompt instead of the generic synthesis reinvoke that was silently dropping the notify dispatch
# --->8---
