# CA-710 — BUG-341 fix review: red-before/green-after + empty-terminal diagnostic

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-341
change_type: bugfix
summary: review-verify opencode blank-first-turn fix (CA-705..709): starvation tests proven red on CA-707 and green on CA-708; add empty-terminal WARN log
# --->8---

## What changed

- `opencode_adapter.go` `emitTerminal`: when `finalText` is empty, log `[opencode] WARN turn_completed with EMPTY finalMessage run=… session=… stopReason=… lastText_len=…` before emitting. Diagnostic-only (zero behavior change) so a future blank has a runner-side breadcrumb (the 5–6 prior fix rounds lacked one).

## Review evidence (this CA)

1. **Red-before proven:** checked out `df539dc2` (CA-707) `opencode_adapter.go` and ran `TestOpencodeUsageUpdateDoesNotStarveText` → `FAIL: got "" want Chao Nam` (usage_update at 10ms reset 8s→150ms). Restored `9ce488a3` (CA-708) code → PASS. So the starvation tests are genuine regression locks, not happy-path.
2. **Green-after:** `go test ./internal/tui/app -count=1` 7.09s PASS; `go test ./internal/runner -run 'TestOpencode(LateChunk|UsageUpdate|ToolUpdate|Thought|NoWait|TextContent|Array)'` PASS; `go build ./internal/runner/... ./internal/tui/...` clean (the `go build ./...` failure is a pre-existing example under `internal/skillpack/flow-pack/golang/...` importing a non-existent `github.com/you/myapp/cmd` — unrelated).
3. **Desktop parity:** `HttpWsRunnerClient.sendTurn` (ts:423-426) uses the same `providerTurnId` filter + close-on-`turn_completed` as TUI `client.go:1252`; runner now emits `turn_completed` with `FinalMessage` before closing, so both surfaces paint the tool-heavy first reply — no Desktop `store.ts` change required.

## Remaining residual risks (honest, non-blocking)

- **8s hard cap:** if the model takes >8s to emit the first `agent_message_chunk` after `session/prompt` returns, the drain ends empty and the turn is blank (now visible via the new WARN log). Raised 8s was a judgement call to keep turns snappy; logs will confirm if it ever fires.
- **150ms quiet truncation:** a multi-part answer with a >150ms inter-chunk gap after the first text would return partial text (not blank) and drop the tail (chunks after `unregisterSession` are discarded). Turn-2 `session/load` may replay the full answer (the "second prompt shows first reply" symptom).
- TUI hydrate-on-empty-terminal is still not implemented — it cannot recover chunks dropped by the runner's `unregisterSession`, so the runner-side wait/mapper fix is the correct layer.

## Prior CA not undone

- `CA-709` array/output_text mapper, `CA-708` wait-on-text-growth, `CA-707` 8s conditional wait, `CA-706` blocking drain, `CA-705` TUI `turnLive`/C2 — all remain; this CA only adds diagnostics + proves the regression locks.
