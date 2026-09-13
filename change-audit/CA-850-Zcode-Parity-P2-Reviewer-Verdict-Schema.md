# CA-850 — CP-62 P-2: reviewer verdicts become schema'd per-AC rows with file:line evidence

# ---8<--- flowpilot:change-ledger
feature_key: zcode-parity
source_doc_id: Task-338
change_type: task
summary: submit_review_outcome gains a verdicts array (ac_id, verdict pass|fail|blocked, evidence[{path,line,excerpt}], note); shape validation at the ONE shared parse point (claude MCP + codex + board handlers all funnel through parseReviewOutcomeInput → cross-provider Case 1); raw rows ride payloadMap verdicts→payload.verdicts to the hub verbatim (no paraphrase on the back-edge); AC coverage = deterministic set diff via ExtractACIDs + ValidateReviewOutcomeVerdicts (missing rows → tool error naming the gap = in-turn reprompt; CP-61 hub-done gate is the fail-closed backstop)
# --->8---

## Why

Reviewer verdicts were a prose blob ({status, summary}); the hub could paraphrase findings and lose file:line evidence on the back-edge; r-requirement and reviewer verdicts used incompatible shapes (CP-62 D-2).

## Change

- `flow-pack/tools/submit-review-outcome.yaml`: `verdicts` input schema (required ac_id+verdict, evidence requires path) + `payloadMap: verdicts → payload.verdicts`.
- `runner/agent_orchestrator.go`: `EvidenceItem`/`VerdictRow` types; `ReviewOutcomeInput.Verdicts`; shape validation inside `parseReviewOutcomeInput` (invalid enum / missing ac_id / missing evidence.path reject the whole tool call — every provider path inherits); `reviewOutcomeToFlowControl` payload carries `verdicts` verbatim.
- `runner/review_verdict.go` (new): `ExtractACIDs` (deterministic AC-N extraction from the locked artifact — the same doc the reviewer read) + `ValidateReviewOutcomeVerdicts` (coverage set-diff; error names missing ACs).
- `flow-pack/agents/reviewer.md`: prompt contract — one verdicts row per AC in the artifact, evidence required for fail/blocked.
- Layering note: per-node expected-AC threading into the three adapter call sites (live per-call coverage enforcement) is the documented follow-up — the helper + parse-layer validation + payload forwarding landed here; until threaded, coverage runs where callers supply the AC list (tests pin all three functions).

## Tests

`runner/review_verdict_test.go` (5 signatures: valid schema, invalid-shape matrix (enum/ac_id/evidence.path), missing-AC reprompt, AC extraction, payloadMap identity). Old `TestCP53ReviewDoneVerdict`/`TestCP61HubDone` suites green; agentpack pack suite green (YAML loads + face parses).

## Providers

Case 1 (shared parse point): `parseReviewOutcomeInput` is the single funnel for claude MCP (claude_permission_mcp.go:232), codex (codex_adapter.go:403), and the board handler (interactive_handlers.go:1580) — verdict shape validation is byte-identical across providers by construction; TestHubForwarding_PreservesRawVerdictsIdentity pins the pack data.

## Prior claims intact

CA-757 (CP-61 hub-done verdict gate) untouched — the no-verdict-no-done contract is preserved and reused as the fail-closed backstop; CA-755 hidden review-loop untouched; legacy callers omitting verdicts parse exactly as before (verdicts optional at parse layer).
