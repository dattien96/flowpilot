# CA-996 — BUG-497: contract store load surfaces read failures

## What changed

`apps/local-runner/internal/changecontract/contract.go`:

- `loadFromDisk` returns `error`: non-ENOENT open failures and mid-file
  read faults propagate through `NewStore` and `OpenStoreReadOnly`.
- Scanner replaced with uncapped `bufio.Reader` — a >4MiB contract line
  (pathological `declared_paths` payload) no longer truncates the load.
- Missing file still yields a usable empty store (first-run contract
  unchanged).

## Invariant

"Unreadable file" is not "no contracts" — declared-path enforcement must
never silently fall open because a load stopped early.

## Tests

`bug497_store_load_errors_test.go`: directory-in-place-of-file → error;
missing file → empty store; oversized line → trailing contract still
loaded.
