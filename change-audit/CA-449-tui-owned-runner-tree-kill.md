---
id: CA-449
feature_key: cli-tui
title: Kill TUI-owned runner process tree on /exit
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-448-tui-statusline-keep-remaining-quota
will_not_undo: CA-445 OwnsRunner=false skip-kill; CA-447 billing?format=credits
```

## Summary

TUI `/exit` left `flowpilot.exe runner serve` and `grok agent` children alive. Remaining quota then came from that stale binary (no CA-447), so TUI never showed %.

## Why leak

1. `runnerboot.spawnRunner` detaches the runner (`CREATE_NEW_PROCESS_GROUP` / `Setsid`) so it outlives the TUI process.
2. Windows `taskkill /PID <pid> /F` had no `/T`, so grok/MCP children became orphans after the listen PID died.
3. Unix sent SIGTERM to the listen PID only, not the process group.

Desktop-reused runners stay untouched (`OwnsRunner=false`).

## Changes

- Windows: `taskkill /PID pid /T /F` with a 3s timeout.
- Unix: SIGTERM process group (`-pid`) then the listen PID.

## Provider impact

TUI shutdown only. Claude/Codex/Grok adapters unchanged. Confirmed provider-agnostic: `taskkillTreeArgs` / `killRunnerByURL` take no `providerKey`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Kill TUI-owned runner process tree on /exit so grok/MCP children do not leak
# --->8---
