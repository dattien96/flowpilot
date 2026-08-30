# CA-657 — Cap Opencode models probe so TUI provider catalog loads

## What

TUI `/provider` showed `No provider catalog loaded yet` on `flowpilot-opencode`. `tui.log` at 09:31:29: `GET /providers` hit the 8s budget (`context deadline exceeded n=0`). Earlier same machine loaded 4 providers in ~4s.

## Why

`DetectProviders` now includes Opencode. `detectOpencodeModels` spawned `opencode models --format json` (unsupported on 1.18.18) then `opencode models` (network catalog) on the request context. That extra probe pushed the scan past the TUI 8s timeout (CA-535). Empty `m.providers` is sticky — `/provider` never retries.

## Fix

- Drop the speculative `--format json` spawn.
- Cap the remaining `opencode models` call at 1.2s. Timeout → `resolveProviderModels` falls back to static `defaultOpencodeProviderModels()`.
- Codex/Claude/Grok detect branches unchanged (append-last).
- TUI: session unlock still 8s (CA-535). If `providers==0` after that, background `GET /providers` (20s) fills the catalog — same pattern as project-catalog retry. `/provider` is no longer sticky-empty.

## Tests

Additive `opencode_models_probe_timeout_test.go`: timeout does not stall, fallback models, fast text parse skips broken default, Grok still does not spawn CLI.

Additive `ca657_providers_catalog_retry_test.go`: empty first load schedules retry and backfill; loaded catalog does not show retry notice.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: Cap opencode models probe so TUI GET /providers stays under 8s
# --->8---
