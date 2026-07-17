# SD-24 Capability Evidence — Claude Code CLI (scoped conclusion)

- Provider/version: Anthropic Claude Code CLI `2.1.211`; model `claude-sonnet-5`.
- Date/environment: 2026-07-17 (local run, UTC timestamps below); scratch workdirs under the session temp scratchpad; three separate probes, each a fresh `--session-id`.

## Probe 1 — acceptance timing (single-shot)

- Command: `claude -p --output-format stream-json --include-partial-messages --verbose --tools "" --no-session-persistence --permission-mode dontAsk --model sonnet "Reply with exactly: FP_TASK257_CLAUDE_LIVE_OK. Do not use tools and do not modify files."`
- Request sent 2026-07-16T23:52:12Z; process exited 2026-07-16T23:52:18Z (~6.2s wall).
- Sanitized transcript: `system:init` → `system:status requesting` → stream `message_start` (real API response header, `ttft_ms:1223`, non-zero cache/token usage) → text deltas → `result` (`subtype:success`, `terminal_reason:completed`, `result:"FP_TASK257_CLAUDE_LIVE_OK"`).
- Finding: `message_start` is a genuine, API-confirmed "provider began responding" signal that arrives before the terminal `result` event — but it is **transient stdout-only**: with `--no-session-persistence` no durable artifact is written to disk (`~/.claude/projects/<hash>/` gained no session file, only the unrelated `memory/` dir). A receipt usable for crash recovery must be queryable *after* the process dies; this one is not, so it cannot back `TurnBridge.Accepted`.

## Probe 2 — durable transcript on a completed turn (session persistence on)

- Command: `claude -p --output-format stream-json --include-partial-messages --verbose --tools "" --permission-mode dontAsk --model sonnet --session-id <uuid> "Write the numbers one to five hundred, one per line, spelled out in full English words..."` (this particular run terminated early via the provider's own content-filter policy, `terminal_reason:api_error` — still a clean terminal outcome for this test).
- Durable artifact found at a **predictable, session-id-keyed path**: `~/.claude/projects/<sanitized-cwd-hash>/<session-id>.jsonl`.
- Transcript content on a **terminated** turn: `queue-operation enqueue` → `queue-operation dequeue` (both timestamped, written before any model output) → `user` message entry → `ai-title` → terminal `assistant` message entry → `last-prompt` entry.
- Finding: the `enqueue`/`dequeue` pair is written to durable storage synchronously, before the network call — it proves the **local CLI accepted the prompt into its own queue**, not that the request reached Anthropic's API. It is an undocumented CLI-internal implementation artifact (no stable contract across `claude-code` versions), so it does not qualify as a provider acceptance receipt either.

## Probe 3 — kill mid-turn, then reconcile + attach (the decisive test)

- Launched a long-generation turn (`--session-id 333...3`, "write a 3000-word story") and, once the durable transcript showed 17+ lines (turn in flight), located the exact OS process by matching `Win32_Process.CommandLine` against the unique `--session-id` value (`claude.exe` PID 11028) and force-killed **only that PID** — not any other running `claude.exe` instance on the machine.
- **Reconcile check**: after the kill, `~/.claude/projects/.../33333333-....jsonl` contained only `queue-operation enqueue` → `queue-operation dequeue` → `user` message → `ai-title` — **no terminal `assistant` message, no `last-prompt` entry**. This is reproducibly distinguishable from Probe 2's completed transcript (which has the trailing `assistant` + `last-prompt` pair). So: **query/reconcile after a crash is possible** — the durable per-session JSONL can tell "dequeued but never finished" apart from "completed" — a real, reproducible reconcile signal, unlike Codex/Grok's single-shot probes which did not exercise this path.
- **Attach check**: immediately after, ran `claude -p --resume 33333333-... "STATUS_CHECK: did you already answer my previous story request, or are you starting fresh? Reply RESUMED_PRIOR_TURN or NEW_TURN_NO_MEMORY..."`. Result: `"NEW_TURN_NO_MEMORY — no story was written previously."` — **confirmed**: `--resume` after a mid-turn crash does not re-attach the in-flight turn; it starts a brand-new turn with no memory of the interrupted request. Respawn = new turn, the same guarantee class as Codex/Grok.

## Scoped conclusion adopted for CP-51

1. **Acceptance receipt: none.** Neither the transient stream `message_start` event (not durable) nor the durable `queue-operation dequeue` line (local-CLI-internal, not a provider ack, undocumented across versions) qualifies as a stable `ReceiptID`. `TurnBridge.Accepted` must not be wired from any Claude adapter call site (`claude_adapter.go:173` `writeUserTurn` remains stdin-write-only, confirmed no `Accepted` call site exists in the adapter).
2. **Query/reconcile after crash: evidenced, not a receipt.** The durable per-session-id JSONL under `~/.claude/projects/<hash>/<session-id>.jsonl` reproducibly distinguishes completed (trailing `assistant` + `last-prompt`) from in-flight/crashed (ends at `ai-title`/`dequeue`, no terminal entry). This feeds recovery's `uncertain` classification as **corroborating signal only** — it is not proof-grade `TerminalEvidence` per SD-24 §6.3a and does not itself terminalize a record.
3. **Attach in-flight: proven negative.** `--resume <session-id>` after a crash starts a new turn with no memory of the interrupted one. No attach capability exists; respawn = new turn.

Net result: Claude reaches the **same safe negative outcome as Codex/Grok** — `unprovable (as a receipt) ⇒ uncertain`, no `Accepted`/attach seam — with the difference that Claude additionally has a reproducible (if non-authoritative) reconcile artifact that Codex/Grok's probes did not establish. This is sufficient to admit Claude into the initial V2 rollout under the same three-outcome handling as Codex/Grok; it is not sufficient to wire any receipt or attach seam.

Scope approval: user requested this live Claude probe on 2026-07-17 now that `claude` CLI is available on this machine, explicitly scoped to Claude only (Gemini remains deferred/unprobed and stays V2-disabled).
