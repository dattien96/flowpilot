# CA-581 — CP-43 P-1 Prompt Fallback for Change Contract

## What

Runner now honors a `[Change Contract]` declared in the **user prompt** even when the AI omits the block in `finalMessage`. Fixes run-204658 C1 where an operator-declared contract in the chat input (`files: calc.go, calc_test.go` with `\r` separators) was ignored, causing `r-contract` reprompt + inferred fallback despite a valid declaration.

## Why

- `prepareChangeContract` parsed only `finalMessage`. Chat C1 places the contract in the user prompt (or the flow planner prompt); the AI echo is a courtesy, not a guarantee. When the AI skips it (Grok in run-204658), the gate treated the turn as undeclared.
- Prompts replayed from NDJSON / Windows clipboard can use `\r` or `\r\n` separators; `ParseDeclaration` split only on `\n`, so a `\r`-joined block was a single unparsable line.
- `interactiveRun.lastPrompt` is display-truncated to 100 chars, so a full contract block would be lost before the gate runs.

## Fix

- `apps/local-runner/internal/changecontract/parse.go:29` — normalize `\r\n` → `\n` and `\r` → `\n` before splitting, so `\r`-joined contracts parse.
- `apps/local-runner/internal/runner/interactive_service.go:353` — add `lastFullPrompt string` to `interactiveRun`; store the complete `in.Prompt` for every non-system turn (system reprompts still seeded when empty). Survives display truncation.
- `apps/local-runner/internal/runner/workflow_store.go:99` — add `LastFullPrompt string json:"lastFullPrompt,omitempty"` to `ProviderSessionState`; round-trips via `sessionStateOf` / `reconstructRunInternal` for restart/resume.
- `apps/local-runner/internal/runner/interactive_service.go:7399` — persist full prompt alongside `lastPrompt` with `isSystemPrompt` guard.
- `apps/local-runner/internal/runner/gate_hook.go:1957` — `prepareChangeContract` gains `prompt` param; tries `finalMessage` first, then `prompt` before `InferFromDiff`. V9-04 "don't overwrite declared" guard also checks the prompt so a prompt-declared contract isn't dropped. Legacy `captureChangeContract` passes `""` for prompt to preserve its tests.
- `apps/local-runner/internal/runner/gate_hook.go:163,633` — both chat and flow gate call sites now supply `rs.lastFullPrompt`.
- `apps/local-runner/internal/tui/app` CA-580 already shipped: `buildGateMessage` no longer shows empty `Options:` (informational, not blocking).

## Tests

Additive only — no legacy suite edits:

- `apps/local-runner/internal/changecontract/parse_crlf_test.go` (new):
  - `TestParseDeclarationCRLF` — `\r\n` block parses.
  - `TestParseDeclarationBareCR` — exact run-204658 `\r` prompt parses.
  - `TestParseDeclarationMixedCRLFAndLF` — mixed separators.
- `apps/local-runner/internal/runner/cp43_prompt_fallback_test.go` (new, provider-agnostic × claude/codex/grok):
  - `TestPrepareChangeContract_PromptFallback_DeclaredFromPrompt` — prompt declared, finalMessage silent → declared + in-scope.
  - `TestPrepareChangeContract_PromptFallback_BareCR` — `\r` prompt path.
  - `TestPrepareChangeContract_PromptFallback_FinalMessageWinsOverPrompt` — finalMessage takes precedence when both declare.
  - `TestPrepareChangeContract_PromptFallback_InferredWhenBothEmpty` — falls back to inferred.
  - `TestPrepareChangeContract_PromptFallback_Run204658Repro` — exact prompt from run-204658 → declared + `contracts.ndjson` persisted + file-exists check.

## Provider impact

Provider-agnostic (Case 1): `ParseDeclaration` and `prepareChangeContract` do not branch on `providerKey`; contract storage and scope diff are shared logic. Tests parameterize `claude`/`codex`/`grok` to prove parity. No per-provider adapter change.

## Verification

- `go test ./internal/changecontract -run TestParseDeclaration -count=1` — 9/9 pass (incl. 3 new CRLF cases).
- `go test ./internal/runner -run TestPrepareChangeContract_PromptFallback -count=1` — 13/13 subtests pass (×3 providers).
- `go vet ./internal/changecontract ./internal/runner` clean (flow-pack asset warnings pre-existing, ignored).
- `go test ./internal/runner -count=1` 8.4s green (full runner suite).
- `go test ./internal/tui/app -count=1` 13.5s green (CA-580 intact).
- `go test ./internal/flowgate -count=1` green.
- Full `go test ./internal/changecontract` has 4 pre-existing darwin failures (`frozen_scope_test.go:46,65,103` `paths_test.go:19` `preflight_test.go:184`) reproduced on clean tree via `git stash` — Windows-separator expectations on darwin, not a regression from this change.

## Residual

- Flow mode: C1 chat fallback does not affect frozen contracts (`preflight_contract_plan` → `frozen_contracts.ndjson` remains the source of truth for coder scope; `r-contract` fallback there is intentionally not flow-driven unless a legacy prompt path is used).
- Plan B "AI must follow declared files" remains an agent-discipline concern; this change only ensures the *declaration itself* is seen. Scope drift (`r-scope`) still enforces file-level adherence after the contract is visible.

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: CP-43
change_type: bugfix
summary: honor Change Contract declared in user prompt (with CRLF/CR) before inferring, preserving full prompt for gate fallback
# --->8---
