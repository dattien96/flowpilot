# SD-24 Capability Evidence — Grok CLI (scoped conclusion)

- Provider/version: Grok CLI (authenticated grok.com); available model list exposed only `grok-4.5`. The requested `composer-2.5` was not available in this client.
- Date/environment: 2026-07-16T21:57:11+07:00; scratch workdir `/tmp`; web search, memory and subagents disabled.
- Command: `grok --single 'Reply with exactly: FP_TASK257_GROK_LIVE_OK. Do not use tools and do not modify files.' --model grok-4.5 --cwd /tmp --permission-mode dontAsk --no-subagents --disable-web-search --no-memory`
- Sanitized transcript: `FP_TASK257_GROK_LIVE_OK`.

## Findings

1. Acceptance receipt: **unproven**. The headless CLI returned final output only; no JSON-RPC `session/update` timing or stable receipt ID was exposed.
2. Query/reconcile after client kill: **not exercised / unproven**. No session-directory query was observed from this probe.
3. Attach after restart: **not exercised / unproven**. This one-shot probe does not establish `session/load` behavior.

## Scoped conclusion adopted for CP-51

The completed live probe is sufficient to make the safe, negative implementation decision for the initial Grok rollout: no observed artifact qualifies as an acceptance receipt, reconcile proof, or restart attach authority. Therefore `TurnBridge.Accepted` has **no Grok call site** in this rollout; post-send delivery remains `send_started` until provider-backed `TerminalEvidence` or operator/recovery `uncertain` handling. This replaces Grok's live-verification placeholders with the explicit outcome **unprovable ⇒ `uncertain`**. It does not claim that Grok lacks those capabilities in every product surface.

Scope approval: user accepted Codex + Grok evidence completion on 2026-07-16; Claude/Gemini remain deferred and are excluded from V2 provider enablement pending their own Task-257 evidence.
