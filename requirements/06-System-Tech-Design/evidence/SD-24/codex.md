# SD-24 Capability Evidence — Codex CLI (scoped conclusion)

- Provider/version: OpenAI Codex CLI `v0.144.5`; model `gpt-5.4-mini`.
- Date/environment: 2026-07-16T21:57:11+07:00; scratch workdir `/tmp`; ephemeral, read-only execution.
- Command: `codex exec --ephemeral --sandbox read-only -m gpt-5.4-mini -C /tmp --skip-git-repo-check 'Reply with exactly: FP_TASK257_CODEX_LIVE_OK. Do not use tools and do not modify files.'`
- Sanitized transcript: the CLI reported `workdir: /tmp`, `model: gpt-5.4-mini`, then returned `FP_TASK257_CODEX_LIVE_OK`.

## Findings

1. Acceptance receipt: **unproven**. The CLI transcript exposes session metadata and final output but no documented per-turn acceptance callback distinct from completion. It cannot derive a stable `ReceiptID`; `TurnBridge.Accepted` must not be wired from this probe.
2. Query/reconcile after client kill: **not exercised / unproven**. This single-turn CLI probe did not expose a documented rollout query artifact.
3. Attach after restart: **not exercised / unproven**. No documented attach result was observed.

## Scoped conclusion adopted for CP-51

The completed live probe is sufficient to make the safe, negative implementation decision for the initial Codex rollout: no observed artifact qualifies as an acceptance receipt, reconcile proof, or restart attach authority. Therefore `TurnBridge.Accepted` has **no Codex call site** in this rollout; post-send delivery remains `send_started` until provider-backed `TerminalEvidence` or operator/recovery `uncertain` handling. This replaces Codex's live-verification placeholders with the explicit outcome **unprovable ⇒ `uncertain`**. It does not claim that Codex lacks those capabilities in every product surface.

Scope approval: user accepted Codex + Grok evidence completion on 2026-07-16; Claude/Gemini remain deferred and are excluded from V2 provider enablement pending their own Task-257 evidence.
