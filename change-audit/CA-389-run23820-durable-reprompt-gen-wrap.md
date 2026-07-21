# CA-389 — run-23820 durable multi-reprompt gen wrap + pending-path doc rules

## Summary

Live ReviewLoop (Grok coder `run-23825` under hub `run-23820`) stranded after a
second flow-gate reprompt: coder stayed `running`, step never DONE, hub never
advanced reviewers. Root cause was **provider-agnostic** durable-intent
plumbing, not the Grok adapter.

1. **Gen wrap (kill path):** `clearIntentFieldsLocked` zeroed
   `pendingGateRepromptGen` after a successful durable reprompt delivery. The
   next gate queue did `gen++` from 0 → 1, reusing idempotency key
   `durable-<run>-reprompt-000…001` already mapped to the completed first
   reprompt turn. `startTurn` short-circuited as replay (`lastTurnID` match),
   cleared the new intent, and never launched turn 3.
2. **Pending-path re-check (amplifier):** BUG-288 #8 holds
   `pendingGateCodePaths` and reuses them as `WrittenPaths` with an empty
   `GitDiff` on remediation turns. `r-ca` / `r-bug` only looked at GitDiff for
   docs, so a prior-turn CA/BUG path did not satisfy the rule and could force
   another durable reprompt into the gen-wrap hole.

Trigger was more common on Grok (malformed mid-line `[Change Contract]` →
first `r-contract` reprompt) but Claude/Codex would stick the same way on any
second durable reprompt.

## Fix

- `clearIntentFieldsLocked`: clear prompt/step/fail counters only; **keep**
  `pendingGateRepromptGen` / `pendingResumeGen` as per-run high-water marks.
  (Hub suppress/park paths that fully abandon intent may still zero gen —
  existing run-9437 tests depend on that; they are not the multi-reprompt
  delivery path.)
- `flowgate`: `HasChangeAuditNoteInPaths` / `HasBugFixDocInPaths`; `code_changed`
  and `bug_fixed` honor WrittenPaths docs when GitDiff is empty.

## Cross-provider parity

**Case 1, provider-agnostic.** Touched symbols:
`clearIntentFieldsLocked`, durable idempotency key construction,
`checkRule`/`Evaluate` for `r-ca`/`r-bug`. Grep: no `providerKey` branch in
these paths. New runner tests use Codex as the registered unit-test provider
(Grok controlled runtime is not in the unit harness); the live reporter was
Grok. Flowgate unit tests take no provider at all.

## additive-tests-only

New files only:

- `apps/local-runner/internal/runner/run23820_durable_reprompt_gen_test.go`
- `apps/local-runner/internal/flowgate/run23820_pending_paths_doc_rules_test.go`

No pre-existing tests edited.

## Verification

- `go test ./internal/runner/ -run 'TestRun23820|TestRun9437|TestDurable|TestBug289' -count=1`
- `go test ./internal/flowgate/ -run 'TestRun23820|CodeChanged|BugFix|ChangeAudit|Contract' -count=1`
- Full `./internal/flowgate` package green

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-43
change_type: bugfix
summary: Keep durable reprompt/resume gen as high-water after clear so a second gate reprompt cannot reuse key …0001 and silently replay the completed first turn (run-23820); honor CA/BUG paths in WrittenPaths on empty-diff pending re-checks so remediation turns do not false-fire r-ca/r-bug.
# --->8---
