---
id: CA-467
feature_key: cli-tui
title: Flow-mode account chip, sub-agent transcript, stop-all
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-466-tui-code-fence-copy
will_not_undo: CA-466 per-fence [copy]; CA-465 You hug box; CA-464 goldmark cache; CA-448 remaining-quota pin
```

## Change

run-91842 flow-mode TUI: account chip was truncated off the statusline; there was no child transcript view; `[stop]` disappeared after the hub `turn_completed`; Ctrl-C / `/stop` only interrupted the parent.

- Pin the active provider-account (and the agents strip) with quota/`[stop]` so flow extras can drop first.
- `/agent <name|runId|main>` and Tab swap the viewport onto that run’s SSE (`StreamLive`). Child send is read-only. Hub orchestration stream stays open.
- `[stop]` / `/stop` / Ctrl-C while a flow is live: `POST .../agent-loop/stop` plus interrupt parent and every child (3s timeout so a stuck runner cannot hang the TUI).
- Orchestration attach uses `StreamLive` (does not exit on `turn_completed`) so step/`agent_graph` updates keep arriving. `StreamWithReconnect` is unchanged for chat turns.

Engine-side: run-91842 `my-reviewer` DID complete (`cohort_member_completed`) but `cohort_join_complete` never logged. That join hang is **not** claimed fixed here.

TUI chrome + `/client/*` stop/stream only. Grok `session/new` and runner cohort join are untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Pin flow-mode account, stream child transcripts, stop-all via agent-loop/stop, keep orch SSE after turn_completed
# --->8---
