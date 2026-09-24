# CA-968 — Devin ACP silent `windsurf-api-key` auth (F-2 no-browser path)

## Change

New `devin_silent_auth.go`:

- `devinAuthenticate(ctx, dispatcher, extraEnv)` — the single auth entry for
  ACP spawns. When the account's launch env already carries a usable
  `windsurf_api_key` it calls `authenticate{methodId:"windsurf-api-key",
  _meta.api_key}` — silent, no PKCE listener, no browser. On an RPC error
  (expired/revoked key) it retries once with `devin-browser`, preserving the
  interactive web-login fallback.
- `devinAPIKeyFromEnv(extraEnv)` — resolves the key in spawn-env order:
  explicit `WINDSURF_API_KEY` in extraEnv → `credentials.toml` under the
  account home (`extraEnv["HOME"]`, via `devinCredentialFilePaths`) →
  ambient `HOME`/`USERPROFILE`/`APPDATA`/XDG candidates (the model probe
  spawns with `devinProcessEnv(nil)` so the ambient store is what its
  process sees).
- `devin_acp.go`: `devinACPAPIKeyAuthParams(apiKey)` builds the params; the
  key rides `_meta.api_key` (exact casing — `apiKey` camelCase falls back to
  PKCE, live-verified).
- `devin_process.go` + `devin_models_probe.go`: both authenticate call sites
  now go through `devinAuthenticate` (`probeDevinModelCatalog` keeps its
  signature via a `probeDevinModelCatalogWithEnv` wrapper — additive for
  existing tests).

## Why

User report: every `devin acp` start opened a browser to
`http://127.0.0.1:65204/callback?code=…` even though the account was already
authenticated — devin.exe logs showed `had_credentials=true` still ran the
full PKCE flow under `devin-browser`. Binary inspection + live probe found
the silent method `windsurf-api-key` accepting the stored key via
`_meta.api_key` ("ACP: API key provided directly via authenticate meta",
~0.5–3s, zero browser). Now an already-authenticated account never pops a
tab; only missing/invalid credentials reach the browser flow — exactly the
requested contract.

## Test

- `devin_silent_auth_test.go` (3 additive tests, red-first):
  key present → `windsurf-api-key`+`_meta.api_key` sent; no key →
  `devin-browser`; silent RPC error → second authenticate with
  `devin-browser` (fallback proven on the wire).
- Live on real account (runner :47789, devin.exe log
  `devin_20260924-164913_13772.log`): `method_id=windsurf-api-key,
  meta_keys=["api_key"]` → `result {}` in ~3s incl. MCP fan-out; no PKCE
  line, no callback listener, no browser. Warm turn then ran
  `session/new` + `model=swe-2-high`/`thought_level=max` → `SA-OK`.
- Live invalid-key probe on the real binary → RPC `-32603 invalid api key`
  (the exact error shape the fallback branch catches); no browser spawned
  during the probe.
- Redaction: `api_key` was already in `devinCredentialShapedKeys` — wire
  logs show `_meta.api_key:"[redacted]"`.
- Focused devin suite (`-run 'Devin|devin'`): all green.

## Risk / parity

- Devin-only authenticate path; Claude/Codex/Grok/Gemini/Opencode untouched.
- Worst case on a flaky key: one extra `authenticate` round-trip before the
  same browser flow as today — bounded by the existing `devinAuthTimeout`
  which now spans both attempts.
- Half-authenticated processes are still never cached (the existing
  `dispatcher.fail + kill` on auth error is unchanged — a browser-flow
  failure after a silent failure kills the process as before).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-439
change_type: feature
# --->8---
