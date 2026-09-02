# BUG-329: Opencode mid-chat model switch fails — session/load returns no sessionId on a fresh acp process

## Metadata

- Document ID: `BUG-329`
- Title: `Opencode mid-chat model switch fails — session/load returns no sessionId on a fresh acp process`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-29`
- Last Updated: `2026-09-02`
- Feature Keys: `ai-providers`
- Parent Documents: [CP-57: Opencode Provider Integration](../../07-Coding-Plan/inprogress/CP-57-Opencode-Provider-Integration.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- Child Documents: `none`
- Related Documents: [CA-680](../../../change-audit/CA-680-opencode-midchat-model-session-load.md), [CA-679](../../../change-audit/CA-679-opencode-config-file-env-and-tui-model-restore.md), [CA-657](../../../change-audit/CA-657-opencode-models-probe-tui-providers-timeout.md), [CA-659](../../../change-audit/CA-659-opencode-local-share-auth-discovery.md), Grok `BUG-324` / run-92955 (same class, inverted transport)
- Replaces: `none`
- Tags: `opencode, acp, session-resume, model-switch, severity-high`

## AI Quick View

### Summary

- Live run-307050: first chat turn on `opencode-go/muse-spark-1.2-contributor` streamed fine; switching the model to `opencode-go/deepseek-v4-flash` mid-chat failed every retry with `turn failed: opencode session/load returned no sessionId`.
- Live probe (opencode 1.18.18): sessions live inside the creating `opencode acp` process instance. `session/load` of a `ses_*` created in process A, issued on a fresh process B, answers **RPC-OK with a config-only result and NO sessionId** (`{"configOptions":[...]}`) — while `session/prompt` on that same loaded id still works (`end_turn`), because sessions persist in the shared `opencode.db`.
- `ensureOpencodeProcess` keyed handles by scope+model+variant+auto (copied from Grok): the mid-chat model change spawned process B, orphaning every session created by process A.

### Root Cause

1. `opencode acp` has no `--model`/`--variant` launch flags; per-turn model rides on `session/set_config_option` (already wired in `applyOpencodeSessionConfig`). But `ensureOpencodeProcess` still respawned on a model/variant/auto key miss (Grok `grokProcessKey` copy, where respawn IS correct because grok acp takes `--model`).
2. `ensureSession` treated the config-only `session/load` result as a hard error (`opencode session/load returned no sessionId`) instead of adopting the requested `ses_*` id.
3. `turnResumeProviderSessionID` had no `lastOpencodeTurnSessionID` fallback (Grok has the `lastGrokTurnSessionID` twin for run-92955).

### Fix (OpenCode-only)

1. `ensureOpencodeProcess`: exact-key hit still wins; on a miss it now **reuses any live handle bound to the same scope** instead of spawning. Model/effort/YOLO changes no longer spawn; an account/scope switch still closes every other scope's process. `opencodeProcessKey` itself is unchanged (old tests keep passing).
2. `ensureSession`: `session/load` with `err==nil` but no `sessionId` adopts the requested resume id when it is a real `ses_*`. A real RPC error still fails the turn — never a silent `session/new` (history loss).
3. `turnResumeProviderSessionID`: opencode falls back to `lastOpencodeTurnSessionID` exactly like the Grok branch; the resume-restore path now seeds `lastOpencodeTurnSessionID` too.
4. `applyOpencodeSessionConfig` still runs after every load so the per-turn model/effort is re-applied to the loaded session.

Claude/Codex/Grok paths untouched; Grok keeps respawn + `session/load` (its acp takes `--model`).

### Validation

- `bug329_opencode_midchat_model_session_load_test.go` — 10 cases: same-scope reuse on model change, variant/YOLO change, exact-key priority, scope-switch close without spawn, SendTurn end-to-end over a config-only load (adopts id, no `session/new`, persists, completes), load RPC error still fails, missing-id without resume still errors, `thread-*` first turn still `session/new`, `turnResumeProviderSessionID` opencode fallback + Grok/Codex parity guards.
- Live probes recorded in this doc: cross-process `session/load` config-only shape; `session/prompt` on the loaded id returns `end_turn`.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: Opencode mid-chat model switch reuses the live acp process and adopts the resumed ses id when session/load omits sessionId
# --->8---
