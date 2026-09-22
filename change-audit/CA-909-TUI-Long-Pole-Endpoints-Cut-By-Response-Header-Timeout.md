# CA-909: TUI Long-Pole Endpoints Cut by ResponseHeaderTimeout

## Summary

- Bug (live repro): `/init all` on DnStudio succeeded ("100 installed"),
  then `scaffold: failed: Post ".../scaffold": net/http: timeout awaiting
  response headers` at exactly T+60s. The runner was healthy — the
  scaffold turn (AI generation + pnpm compiler gate) legitimately holds
  response headers for minutes.
- Root cause: the TUI transport sets `ResponseHeaderTimeout: 60s` (added
  to guard a hung-but-accepting runner). It applies to every request,
  including endpoints that synchronously run minutes-long work —
  `POST .../scaffold` (60min ctx budget) and `POST .../engine/init`
  (5min ctx budget). `http.Client.Timeout` is 0, so the transport header
  timeout was the effective killer.
- Fix: `Client` gains a second transport (`longHTTP`) cloned from the
  normal one with `ResponseHeaderTimeout = 0`. `InitEngine` and
  `DispatchScaffold` ride it via `postJSONLong`; every other RPC keeps
  the 60s hung-runner guard. Per-call ctx deadlines remain the real bound.

## Files

- `apps/local-runner/internal/tui/client/client.go`
  (`longHTTP` client, `postJSONLong`, `methodJSONOn(hc, ...)` extraction,
  InitEngine + DispatchScaffold routed to long transport)
- `apps/local-runner/internal/tui/client/timeout_test.go`
  (new TestDispatchScaffold_NotCutByResponseHeaderTimeout — shrinks the
  normal transport's header timeout to 50ms against a 250ms-delayed
  server: Health still times out (guard intact) while DispatchScaffold
  succeeds — red before, green after)

## Out of Scope

- Server-side alternative (202 + status polling) would remove the
  long-blocking request entirely; bigger contract change, deferred.
- `gitnexus` MCP unreachable during this change (tool listing failed);
  impact assessed manually: `Client` field addition, private helper
  extraction, two call sites — leaf-level.

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: live-repro-tui-init-all-scaffold
change_type: bugfix
summary: route engine-init/scaffold dispatch over a transport without the 60s ResponseHeaderTimeout
# --->8---
