---
id: CA-471
feature_key: cli-tui
title: Revert Grok /model new-session copy (keep history)
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-470-tui-unwrap-markdown-fence
will_not_undo: CA-445 Grok session/new cwd+mcpServers only; CA-210/Task-210 cross-account RelocateSessionFile
```

## Change

Uncommitted CA-452 dropped Grok ACP resume on mid-chat `/model` (`session/new`), which wiped provider history. That runner change is reverted (not shipped).

TUI `/model` no longer claims "Grok starts a new provider session". Copy matches Claude/Codex: next prompt uses this model. History stays on `session/load` like same-provider account switch (`RelocateSessionFile` + resume). Grok model-switch-with-history needs a later design (session-dir rewrite or handoff), not a blank `session/new`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Revert Grok /model new-session copy so mid-chat model change does not drop ACP history
# --->8---
