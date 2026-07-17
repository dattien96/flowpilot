# CA-340: Task-257 Claude capability evidence — V2 gate enabled

## Scope

Run the deferred Claude leg of the Task-257 capability-evidence spike now that `claude` CLI (2.1.211) is available locally. Scoped to Claude only; Gemini stays deferred/V2-disabled per user request.

## Changes

- Live probes against Claude Code CLI: single-shot acceptance-timing probe, durable-transcript probe on a completed turn, and a kill-mid-turn probe (exact-PID match via `Win32_Process.CommandLine` on the probe's unique `--session-id`, so only the throwaway probe process was terminated) followed by a `--resume` attach attempt.
- Findings: no stable acceptance receipt (transient `message_start` isn't durable; durable `queue-operation dequeue` is CLI-internal/undocumented); a reproducible but non-authoritative reconcile artifact exists (`~/.claude/projects/<hash>/<session-id>.jsonl` distinguishes completed vs in-flight/crashed transcripts); attach is proven negative (`--resume` after a kill starts a new turn with no memory of the interrupted one).
- Net result matches Codex/Grok's safe negative class (`unprovable ⇒ uncertain`, no `Accepted`/attach seam) — sufficient to admit Claude into the initial V2 rollout under the same three-outcome handling.
- `apps/local-runner/internal/runner/dispatch_live.go`: `providerV2Enabled` moves `"claude"` into the default-enabled bucket alongside `codex`/`grok`/`fake`; only `"gemini"` remains gated behind `FLOWPILOT_DISPATCH_V2_PROVIDERS`.
- Tests: `cp51_tasks_test.go` — `TestProviderV2ClaudeGeminiDisabled` → `TestProviderV2GeminiDisabled_ClaudeEnabled`; `TestCapabilityEvidence_CodexGrokNoAcceptedSeam` → `TestCapabilityEvidence_CodexGrokClaudeNoAcceptedSeam`; `TestCapabilityEvidence_ClaudeGeminiAllowListOptIn` → `TestCapabilityEvidence_GeminiAllowListOptIn`.
- Docs: new `requirements/06-System-Tech-Design/evidence/SD-24/claude.md` (full probe transcript + conclusion); SD-24 §6.3 matrix row + §6.3a text updated; CP-51 ledger row `CE-CL/GEM` split into `CE-CL` (✅) and `CE-GEM` (☐), `RC`/P-0/Q-2/Touched-Areas text updated; Task-257 doc Summary/Current Ask/Acceptance-Check/Completion-Notes updated to reflect Claude done, Gemini still open.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-257
change_type: feature
summary: Claude Task-257 capability evidence recorded; providerV2Enabled defaults claude to V2 three-outcome enabled, Gemini remains deferred
# --->8---
