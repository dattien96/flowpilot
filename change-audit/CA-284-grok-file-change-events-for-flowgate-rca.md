# CA-284: Grok File-Change Events For Flowgate r-ca

## Scope

Task-212 DOD-2: normal Grok chat did not fire post-turn flow gates (r-ca) after code edits because `EventFileChanged` was not emitted when mutation kind/path lived only under `_meta.x.ai/tool` or `rawInput`.

## Root Cause

- `r-ca` uses `WrittenPaths` from `fin.ChangedFiles`, collected only from `EventFileChanged`.
- Live Grok ACP (gate-sandbox 2026-07-11, `search_replace` on `calc.go`):
  1. `tool_call` / mid-flight `tool_call_update` carry `kind=edit` + `locations` / `rawInput.file_path`
  2. Final `status=completed` frame is **stripped** — only `content`/`rawOutput`
- Mapper only emitted file_changed on completed frames with kind+path → always missed live edits.
- First fix (meta/rawInput on the completed frame alone) was insufficient without **toolCallId correlation**.

## Changes

- `grok_event_mapper.go`: mutation kind/path helpers + `grokCorrelateToolNotification` / remember / enrich
- `grok_adapter.go` `SendTurn`: per-turn `pendingToolCalls` cache before `mapGrokNotification`
- Tests: mapper meta/rawInput cases, live-shaped correlation, normal-chat r-ca

## Verification

- `go test ./internal/runner -run 'MapGrokToolCallUpdate|GrokToolCallCorrelation|GrokNormalChatR|GrokMapperFileChange'` — PASS
- Live desktop re-verify after **restart runner** (must load new binary)

## Follow-ups

- User live: normal Grok chat, edit `*.go`, no new CA → expect timeline `⚠ Flow gate: code changed but no change-audit note` and/or auto reprompt turn

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-212
change_type: bugfix
summary: Grok tool_call_update maps meta/rawInput file mutations to EventFileChanged so flowgate r-ca WrittenPaths populates
# --->8---
