# CA-1145 — BUG-601: live SSE lanes starve every JSON call (boot freeze)

- **Bug:** BUG-601 (live CP-03 watch). Chromium pools ~6 HTTP/1.1
  connections per origin; the desktop can hold up to six long-lived SSE
  lanes against the runner at once (mux `streamRunUpdates` + `streamRun`
  for focused/main/history/orchestration consumers). `lsof` showed all six
  renderer connections to `127.0.0.1:4317` ESTABLISHED and never
  completing — so `GET /client/projects` queued indefinitely and the boot
  overlay froze forever on "Connect to runner & load projects" even though
  the runner answered `curl` in <1s and the health badge still read green
  from its last success.
- **Fix:** `HttpWsRunnerClient` now derives `streamBase` — the loopback
  alias of `base` (`127.0.0.1` ↔ `localhost`). Chromium keys pools per
  origin, so SSE lanes (`openStream`, `streamRunUpdates`) get their own
  six-slot pool while `getJSON`/`postJSON`/`putJSON` keep the API pool to
  themselves; both aliases hit the same listener. `getJSON` also gains a
  30s `AbortSignal.timeout` so a stalled connection degrades into a
  retryable failure (boot Retry) instead of pinning the overlay forever.
- **Contract preserved:** stream endpoints, event shapes, reconnect and
  abort semantics unchanged — only the request origin differs. When `base`
  is neither loopback spelling the client behaves exactly as before
  (`streamBase === base`). POST/PUT callers that already passed explicit
  timeouts are untouched.
- **Tests:** typecheck clean; existing stream tests
  (`streamRunUpdates.test.ts`, `streamRunUpdatesBug482.test.ts`,
  `muxLoop`, `muxUpsertBug479`) still pass — they hit the same listener
  through either alias.
