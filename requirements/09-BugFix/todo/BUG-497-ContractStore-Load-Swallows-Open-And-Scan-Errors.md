# BUG-497 — changecontract loadFromDisk swallows open + scanner errors → declared-path enforcement silently blind

## Status
FIXED — unit-verified (red→green); live leg verified 2026-09-28
(:4322 fp-live5, contracts.ndjson fault drill via
`GET /client/workflow-runs/{runId}/steps/{stepId}/contract`).

## Severity
Medium — enforcement degradation, fail-open direction

## Symptom

`changecontract.Store.loadFromDisk`:

```go
f, err := os.Open(s.filePath)
if err != nil {
    return // missing file is not an error — first run
}
```

(`internal/changecontract/contract.go` ~:70)

Two swallows:

1. **Any** `os.Open` failure is treated as "first run" — a permission-denied
   or otherwise unreadable `contracts.ndjson` produces a silently empty
   store, not a missing-file empty store.
2. `scanner.Err()` unchecked — a mid-file read fault or a >4MiB line stops
   the load; contracts after that point are silently absent.

Downstream, `Get`/`GetLatestForRun` report "no contract" for runs that do
have one → declared-path checks fall back to inferred scope (fail-open:
the restriction the contract exists to enforce silently disappears).

## Root cause

`loadFromDisk` returns nothing; `NewStore` and `OpenStoreReadOnly` call it
without observing failure. "Missing file" (legit empty) and "unreadable
file" (corrupt state) are conflated — the BUG-485/491 class contract
requires distinguishing them.

## Fix (implemented)

`loadFromDisk` returns `error`:

- `os.IsNotExist` → nil (true first-run empty store, unchanged).
- other open errors → propagated by both `NewStore` and
  `OpenStoreReadOnly` (callers already treat a returned error as degraded
  — better than a silently-empty authoritative store).
- `scanner.Err()` → propagated so a partial load is never presented as
  complete.

## Tests

- `bug497_contract_store_load_errors_test.go`
  - RED: `contracts.ndjson` replaced by an unreadable file/dir →
    `OpenStoreReadOnly` returns error instead of an empty store.
  - RED: load terminated mid-file (injected read error via helper seam)
    → error, not partial contents.
  - Missing file → empty store, nil error (unchanged).
  - Healthy multi-line file → all contracts loaded (unchanged).

## Live verification (R17 addendum, 2026-09-28)

Via `GET /client/workflow-runs/{runId}/steps/{stepId}/contract` on :4322
(workspace `/tmp/fp-live5`, run `run-40835` resumed into memory):

- Baseline (no contracts.ndjson): 200 `found:false` — empty store.
- **Unreadable leg**: `contracts.ndjson` replaced by a directory →
  **500 `contract_store_unavailable`**: `read …/contracts.ndjson: is a
  directory` — the load error reaches the HTTP surface instead of
  masquerading as an empty store.
- **Oversized-line leg**: file containing a ~5 MiB declared_paths line
  followed by a valid contract row → 200 `found:true` with the tail
  contract (`run-40835`/`cp_reader`) fully returned, including scope-diff
  computation — pre-fix scanner cap would have truncated the load and
  silently dropped it.
- File removed after the drill; workspace restored.
