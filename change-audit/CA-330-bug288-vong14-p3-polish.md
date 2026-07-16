# CA-330: BUG-288 Vòng 14 P3 polish (R14-01…R14-04)

## Scope

Close four residual P3 findings from Claude re-review of the Grok Vòng 13 land: skip-flag leak, finishTurn comment noise, doc ledger pointer, durable marker secret init independent of NDJSON-only construction.

## Changes

- `cohort_stall.go`: `stalledSkipCause` only when `cancel != nil` (R14-01).
- `interactive_service.go`: concise R13-02/R14-02 comment; `initRunMarkerSecretForStore` on service construction (R14-04).
- `flow_context_handoff.go`: `initRunMarkerSecretForStore` + UserConfigDir fallback for non-NDJSON.
- `local_file_session_store.go`: `dataDir` + `DataDir()`.
- Doc BUG-288 § Vòng 14 Fixed + CA-329 pointer (R14-03).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-288
change_type: bugfix
summary: close Vòng 14 R14-01..R14-04 — skip flag leak, finishTurn comments, marker secret service-wire init, doc CA-329 pointer
# --->8---
