---
id: CA-912
title: ACP_BACKEND env leak makes `devin -p` ignore on-disk credentials
type: BugFix
feature: skill-anchored-init
date: 2026-09-22
status: done
---

## Context

Manual scaffold reproduction (`POST /client/projects/{id}/scaffold` against a
hand-spawned runner) failed in 1.7s:

```text
stderr.txt: Error: Not logged in. Run `devin auth login` to authenticate.
```

while the same workspace/provider/model had run fine at 11:26 under the
TUI-spawned runner.

## Root cause

`ACP_BACKEND` is exported by Windsurf into shells it owns (this agent's shell,
integrated terminals). The env var switches every spawned `devin` process —
**including one-shot `devin -p`** — to "host is the sole source of credentials"
mode, so it skips the on-disk `%APPDATA%\devin\credentials.toml` store
entirely. Verified live:

- `devin -p` with `ACP_BACKEND` set → `Not logged in` (instant exit 1).
- `env -u ACP_BACKEND devin -p` → authenticated, real agent output, exit 0.
- `devin auth status` under `ACP_BACKEND` → `Not logged in`; without → uses
  the file. Env-dependent, credential-independent.
- Runner `devin acp` handshake (`authenticate{devin-browser}`) still succeeds
  either way via PKCE — which is why interactive chat worked while `-p`
  starved, and why the earlier TUI run (spawned from PowerShell, no
  `ACP_BACKEND`) succeeded while a runner spawned from a Windsurf-owned shell
  failed.

Anyone running `just chat-dev` / the TUI from a Windsurf-integrated terminal
hits this: chat works (ACP always authenticates via handshake), scaffold dies
immediately. `devinProcessEnv` already strips `DEVIN_*`/`WINDSURF_API_KEY` for
the same class of leak; `ACP_BACKEND` was missed, and `getEnvForExecution`
(the one-shot path used by scaffold) stripped nothing.

## Change

- `internal/runner/runner.go` `getEnvForExecution`: when
  `providerKey == devin`, drop `ACP_BACKEND` from the inherited env (both
  ambient and account-scoped branches). Scoped to devin only — mirrors the
  `agyFilteredEnv` precedent for gemini; other providers keep the var.
- `internal/runner/devin_process.go` `devinProcessEnv`: strip `ACP_BACKEND`
  from `devin acp` launches too — with the flag present the server adopts the
  sole-source policy and `authenticate` never persists refreshed credentials,
  which is what keeps `credentials.toml` fresh for `-p` turns.
- New helper `withoutEnvKeys(env, keys...)` in `runner.go`.

## Reproduction / verification

- Red→green: `TestGetEnvForExecutionDevinStripsACPBackend` (ambient + scoped
  branches stripped; codex untouched), `TestDevinProcessEnvStripsACPBackend`.
- End-to-end: rebuilt runner spawned from an `ACP_BACKEND=windsurf` shell on
  `:4318`, same scaffold POST → `devin -p` (PID 14288) authenticated and began
  the real scaffold turn instead of the 1.7s auth failure.

## Test

- `internal/runner/ca912_devin_acp_backend_env_test.go` (new, additive).

## Provider parity

Devin-scoped by construction: the new filter only activates for
`providerKey == devin`; the test asserts `codex` keeps `ACP_BACKEND`.
`devinProcessEnv` only feeds `devin acp` spawns. No Claude/Codex/Grok adapter
or stream touched.

## Notes

- The earlier interactive-run runner exit (`os.Exit(0)` via `/system/shutdown`)
  remains a separate incident; CA-911 telemetry identifies the caller on the
  next occurrence.
