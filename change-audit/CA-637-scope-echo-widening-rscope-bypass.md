# CA-637 — r-scope bypass: model self-widening Change Contract echo (run-232492)

## What

run-232492 (CP-43 B14/C4 live, chat + Grok): user prompt declared `files: user.go`
but the model's final message echoed a `[Change Contract]` whose `files:` covered
**every file it touched** (`calc.go, calc_test.go, user.go, user_test.go,
change-audit/CA-914-*.md, requirements/08-Task/...`). `prepareChangeContract`
computed scope drift against the model's self-widened echo, so
`actual_touched \ declared_scope` was empty → `r-scope` never fired → the
out-of-scope `calc.go` edit sailed through with only an unrelated `r-tamper`
warn. The stored contract's `intent` was also the model's paraphrase, not the
user's.

## Why

`prepareChangeContract` (gate_hook.go) tried `ParseDeclaration(finalMessage)`
first and, when the model echoed a declaration, used it as the *only* scope
authority (CA-581 "FinalMessageWinsOverPrompt"). The model's echo is a
self-claim: it can widen `declared_paths` to cover whatever it touched, which
silently defeats Task-185's scope-drift gate. Chat B13/B14 (warn/block paths)
became untestable in the live path.

## Fix

`apps/local-runner/internal/runner/gate_hook.go` (`prepareChangeContract`):

- Scope authority for `r-scope` is now the **user-declared scope** — this turn's
  prompt declaration first, else a previously persisted declared contract
  (saved from the user's prompt), else the model echo, else inferred.
- The model's final echo can **never widen** the user-declared scope.
- The declared contract persisted to `contracts.ndjson` is the
  **user-authoritative scope** (`scopeC`), so a follow-up turn's echo cannot
  re-widen it either (multi-turn durability).
- `out.contract` (canonical-head / final-message precedence) is unchanged to
  preserve the legacy `FinalMessageWinsOverPrompt` contract (CA-581).
- `captureChangeContract` passes `prompt=""` → unaffected.

## Tests

Additive only — legacy suite untouched (`cp43_prompt_fallback_test.go` intact).

- `apps/local-runner/internal/runner/run232492_scope_echo_widening_test.go` (new):
  - `TestRun232492_EchoCannotWidenUserScope` — exact run-232492 shape: prompt
    `user.go`, echo widens → `calc.go`/`calc_test.go` still flagged; stored
    contract keeps `[user.go]`.
  - `TestRun232492_EchoCannotWidenScopeAcrossTurns` — turn 2 (user silent, model
    re-echoes widened) still flags out-of-scope from the persisted user scope.
  - `TestRun232492_InScopeEchoNoDrift` — no over-flagging when echo stays inside
    the user scope.
  - `TestRun232492_FinalOnlyScopeUsedWhenNoUserPrompt` — echo scope still applies
    when the user declares nothing (old behavior preserved).
- Provider-agnostic (Case 1): `prepareChangeContract` has no `ProviderKey`
  branch (grep 0 hits); all new tests parameterized `claude`/`codex`/`grok`.

## Verification

- `go test ./internal/runner -run 'TestRun232492|TestPrepareChangeContract|TestScopeDiff|TestChatGate|TestCanonicalHead|TestChangeContract|TestFlowCoderUsesFrozenScope|TestDependence' -count=1` → green (19s).
- `go test ./internal/changecontract/... ./internal/flowgate/... -count=1` → green.
- `go vet ./internal/runner ./internal/changecontract ./internal/flowgate` → clean (pre-existing flow-pack asset warnings only).
- Full `./internal/runner` suite: 11 pre-existing failures reproduced **identically on the clean tree** (`git stash` + rerun): Firebase/Supabase (env/network), `Run147126_*`/`FirstCoderContext*`/`RootFlowEngine*`/`ResumeFlow*`/`FinalizePartial*` (env), `TestLiveChatRefreshesLedgerBeforeFeatureHistoryInjection` (TempDir cleanup flake). None introduced by this change.

## Residual

- Canonical Head `intent`/`declared_paths` still derives from the model echo
  (`out.contract`) because the legacy `FinalMessageWinsOverPrompt` test pins
  final-message precedence — a separate head-pollution concern, not addressed
  here.
- Frozen-contract flow writers already use `FrozenContractScopeDrift` (not this
  path) — unaffected, verified by `TestFlowCoderUsesFrozenScopeInsteadOfFinalMessage`.

Will not undo: CA-581 prompt fallback, CA-582 binary scope, CA-613 audit prefers
declared contract, CA-627 ledger exemption, CA-634/635 frozen drift.

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: CP-43
change_type: bugfix
summary: user-declared prompt scope is authoritative for r-scope; a model echo can no longer widen declared scope to bypass scope-drift (run-232492)
# --->8---
