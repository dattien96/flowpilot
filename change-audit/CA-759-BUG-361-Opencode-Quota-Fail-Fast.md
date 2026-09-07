# CA-759 — BUG-361: OpenCode quota signals fail fast with stable copy

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-361
change_type: bugfix
summary: OpenCode quota/billing RPC errors and stopReasons map to a stable usage-limit turn_failed instead of retrying or blank-completing
# --->8---

## Live repro (run-208380, TUI Task Harness on gate-sandbox)

- Exhausted `opencode/muse-spark-1.3-contributor-free`: scout `preflight_contract_plan` RUNNING / Thinking 2m+ with no stream and no fail. Loop `running`; parent watchdog correctly suppressed by the live child.
- Code gaps found by reading (no live ACP frame was capturable — runner went down before re-query): `opencode_event_mapper.go` had zero quota tokens; `opencodeStopReasonToEvent` defaulted every unknown stopReason to Completed; `isProviderUsageLimitError` had no underscore/dash ACP shapes (`rate_limited`, `quota_exceeded`), so a returned quota error classified recoverable.

## Change (adapter + shared classifier, additive only)

- `interactive_service.go` `isProviderUsageLimitError`: ACP tokens plus live 1.18.29 gpt-5.4-nano JSON-RPC `-32603` copy (`Internal error: No payment method. Add a payment method here: …/billing`) via `no payment method` / `add a payment method`. Claude/Grok strings verbatim.
- `opencode_event_mapper.go`: new `opencodeIsQuotaStopReason` (unambiguous billing/quota tokens only); `opencodeStopReasonToEvent` maps those to `turn_failed`. Unknown reasons still complete — default unchanged.
- `opencode_adapter.go` `SendTurn`: quota RPC error → one `OpenCode usage limit reached: …` event (`Recoverable: false`) + nil (codex pattern, BUG-259-safe); non-quota errors return unchanged. `emitTerminal`: quota stopReason carries the same copy.

## Live ACP probe (opencode 1.18.29, 2026-09-07)

- `opencode-go/deepseek-v4-flash`: `session/prompt` returns `{stopReason: end_turn}` in 3.1s — healthy path unchanged.
- `opencode/gpt-5.4-nano`: `session/prompt` JSON-RPC error in 1.6s: `code=-32603 message="Internal error: No payment method. Add a payment method here: …/billing" data.errorName=APIError`. No session/update text. Classifier now maps this; SendTurn fails fast.
- `opencode/muse-spark-1.3-contributor-free`: `session/prompt` never returns (15s probe, 0 post-setup frames). OpenCode log on the hung TUI run was `AI_APICallError: Rate limit exceeded` but ACP does not surface it. Residual F-2 unchanged — `/stop` and switch off muse-spark.

## Deliberately not changed

- No turn-level timeout for a never-returning `session/prompt` (doc F-2): no signal distinguishes it from healthy slow generation, and a blind bound collides with the CA-708 activity budget. Residual: a truly outstanding prompt still hangs until `/stop`.
- No hub_stall change (correctly suppressed by live child), no TUI chrome, no CP-61/CA-758 paths.

## Tests

- New `bug361_opencode_quota_hang_test.go`: classifier table (incl. live nano billing copy; generic `Internal error: boom` stays healthy), stopReason table, quota RPC fail-fast, live no-payment-method RPC fail-fast, non-quota RPC passthrough, quota stopReason fail.
- R1: `TestOpencodeProcessEnvIsolatesWindows` fails identically on the clean tree via `git stash` (Windows `OPENCODE_CONFIG` path expectation, zero overlap) — pre-existing, untouched. All other tests in the parity run green, incl. `TestOpencodeActivityKeepsBudgetPastInitialCap` (no false quota fail on slow generation).
- R2: Case 2 — shared classifier extended additively; OpenCode-only mapping code; Claude (`TestClaudeUsage*`, `TestClaudeEventMapper*`), Codex (`TestCodexEventMapper*`), Grok (`TestIsProviderUsageLimitError*`, `TestGrok*`) suites run green.

## Prior CA intact

- CA-061/067/068/069/070 Claude limit copy and Grok 402 classifier untouched and green; CA-708 activity budget untouched and green; CA-712/713 denied-turn replay untouched (quota-Failed short-circuits before replay).
