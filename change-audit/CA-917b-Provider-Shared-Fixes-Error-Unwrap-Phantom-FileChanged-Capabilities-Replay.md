---
id: CA-917b
title: Shared/provider fixes — JSON-RPC error.data unwrap, opencode phantom file_changed, admin capabilities, grok retry replay dedup, store pollution guard, opencode catalog filter
type: BugFix
feature: ai-providers
date: 2026-09-23
status: done
---

## Context

Second wave of the `cp_live_test` bug-fix pass (Cluster B): provider-shared and
opencode/grok-specific defects captured live — BUG-381, BUG-382, BUG-383,
BUG-384, BUG-385, BUG-431.

## Change

- `internal/runner/sessions.go`
  - `jsonRpcErrorMessage` (BUG-381): unwraps `error.data.message` /
    `error.data.http_status` before falling back to the generic top-level
    `error.message`. ACP servers (grok/opencode/devin) return
    `{"code":-32603,"message":"Internal error","data":{"http_status":402,
    "message":"API error (status 402 Payment Required): …"}}` — the real
    reason now reaches `turn_failed` instead of "Internal error" after ~3
    wasteful retries.
- `internal/runner/opencode_event_mapper.go`
  - `mapOpencodeToolCallUpdate` (BUG-382): non-terminal statuses
    (`in_progress`/`pending`/`running` and any other non-success/failed)
    emit nothing — previously surfaced as `tool_completed`. `file_changed`
    is emitted only when the mapped status is `success` — a denied write
    (status `failed`, "user rejected permission") no longer forges a
    file_changed that poisoned `WrittenPaths` and tripped r-ca/r-contract
    gates.
- `internal/runner/provider_registry.go`
  - BUG-383: opencode and devin registrations evaluated
    `Capabilities()` on zero-value adapters — `ApprovalEvents`/`Mcp` reported
    false because `mcpServer` is wired per turn. Both now register static
    capability literals matching the wired provider truth (same pattern as
    the codex/claude/grok literal registrations). Grok's literal gains
    `Mcp: true` (same surface, same lie — live-verified in cp46).
- `internal/runner/interactive_resume.go`
  - `dedupeRetryDuplicatedTurnStarts` (BUG-384): collapses consecutive
    identical-prompt `turn_started` events in provider-history replay. Every
    SendTurn retry re-issues `session/prompt`; Grok persists each attempt as
    a `<user_query>` frame — surplus frames previously surfaced as phantom
    user turns leaking the composed prompt (internal MCP reinforcement
    scaffolding) into the timeline and cross-provider handoff. Applied in
    `seedGrokTranscriptFromDisk` across the concatenated session history
    (covers retries split across rotated session files). Keep-first
    preserves overlay alignment; a genuinely re-typed identical prompt is
    still restored via `prependMissingPromptOnlyEvents`.
  - `appendTranscriptReplayEventsOpt` (BUG-384): the synthetic tail closing
    a non-terminal seeded transcript is `turn_failed` when the run's status
    is failed/cancelled — a 402 turn previously replayed as `turn_completed`,
    masking the failure.
- `internal/runner/provider_accounts.go`
  - `providerAccountsConfigPath` (BUG-385a): under `go test`
    (`runningUnderGoTest`) with no env override, resolves an isolated
    `os.TempDir()/flowpilot-go-test/` path instead of the machine-global
    `~/Library/Application Support/FlowPilot/provider-accounts.json`. A
    bare-service test reaching a store read+sync-write can no longer leak
    synthetic `acct-*` fixtures into the real store. Explicit
    `FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH` overrides still win.
- `internal/runner/handoff_context.go`
  - `renderConversationTurn` (BUG-385b): an assistant-less turn no longer
    appends `[turn truncated due to handoff size limit]` while reporting
    `truncated:false` — failed turns were labeled truncated, misinforming
    the receiving provider about conversation fidelity.
- `internal/runner/opencode_models_verbose.go`, `internal/runner/runner.go`
  - `opencodeIsChatCapableModel` (BUG-431): filters non-conversational model
    families (deep-research / Interactions-API-only, embedding, veo, lyria,
    tts, live-*, computer-use) from the opencode catalog in both the verbose
    and plain `opencode models` parsers — same pattern as the existing
    `deepseek-v4-flash-free` skip. The multi-family account's picker default
    had landed on `google/deep-research-max-preview-04-2026`, failing every
    turn with "This model only supports Interactions API".

## Tests

- `bug381_jsonrpc_error_data_test.go` — data.message unwrap, top-level
  fallback, http_status synthesis.
- `bug382_opencode_phantom_filechanged_test.go` — denied write → no
  file_changed; in_progress → no tool_completed; completed write still emits
  both (regression guard).
- `bug383_admin_capabilities_test.go` — opencode + devin registrations report
  approvalEvents/mcp true.
- `bug384_grok_retry_replay_test.go` — 3-attempt retry frames collapse to the
  2 durable prompts; failed run replays tail as `turn_failed`.
- `bug385_store_pollution_truncation_test.go` — config path never resolves
  the real store under `go test`; assistant-less turn carries no truncation
  marker.
- `bug431_opencode_catalog_filter_test.go` — plain + verbose parsers drop
  non-chat families, keep chat models.

## Provider parity

- `jsonRpcErrorMessage` is the shared ACP error path — benefits all ACP
  providers (grok 402 evidence; opencode/devin share the JSON-RPC envelope).
- opencode mapper/registry changes are provider-specific; grok + devin mappers
  already carry equivalent correlation/terminal-status handling (BUG-436 work
  in CA-916b). Claude/codex unchanged — different transport.
- `dedupeRetryDuplicatedTurnStarts` is applied on the grok replay path only;
  the opencode path replays from the durable turn log (no provider-owned
  transcript file), claude from rollout files with different frame shapes.
- The `providerAccountsConfigPath` test guard is harness-only — zero runtime
  effect outside `go test`.

## Live evidence

- cp46: `error.data` shape `{"http_status":402,"message":"API error (status
  402 Payment Required): Grok Build usage balance exhausted"}` — the exact
  payload the unwrap now surfaces.
- cp57: denied write arrives as `tool_call_update` `status:"failed"` with
  "user rejected permission" — now emits `tool_completed(failed)` without
  `file_changed`.
- cp46: 6 `<user_query>` frames for 2 typed prompts; handoff
  `includedTurnCount:4` — dedupe collapses to the durable count.
- cp46/ui: `google/deep-research-max-preview-04-2026` in the live 87-model
  opencode catalog (verified on this machine's cache); `acct-grok` record in
  the real provider-accounts.json with a `TestRun20332*` TempDir home.

## Risks / follow-ups

- `opencodeIsChatCapableModel` is a conservative blocklist — a future
  chat-capable model named e.g. `*-live-*` would be filtered; acceptable vs.
  the deterministic dead-by-default failure it prevents.
- Retry dedupe keeps the FIRST identical frame; if a provider ever embeds a
  per-attempt token inside the composed prompt text, frames won't match and
  stay separate (safe — no false collapse).
- The test-store guard means a test wanting the REAL store must set the env
  override — verified no existing test asserts the default path.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-381
change_type: bugfix
summary: Provider-shared fixes — error unwrap, phantom file_changed suppression, capability gating, replay; feature migrated provider-runtime -> ai-providers (BUG-442)
# --->8---
