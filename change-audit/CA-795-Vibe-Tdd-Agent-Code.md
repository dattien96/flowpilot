# CA-795 — vibe-sprint tdd is agent.code like harness test_signatures

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: refactor
summary: vibe-sprint tdd behavior is agent.code; freeze binds tdd first then coder; signature prompt unchanged
# --->8---

## Why

Operator: make vibe implement chain consistent with task-harness. CA-794 already attached `prompts/test-signatures.md`. Remaining gap: `tdd` was `agent.delegate`, so freeze bound `coder` and walked context→tdd via CA-785 workaround.

## Change

- `flows/vibe-sprint.yaml` `tdd.behavior: agent.code` (keep id `tdd`, tester, signature prompt, `tdd_signatures` binding, lifecycle `reinvoke`).
- `resolveFreezeWriterTarget` hops `context.produce` → first `agent.code` = `tdd`. Sibling bind still copies the frozen contract to `coder` (Task-293).
- Approved one-line update of CA-794 pack assertion `delegate` → `agent.code` (same-session file). CA-785 fixture tests untouched.

## Tests

New `ca795_vibe_tdd_agent_code_test.go` (pack + freeze-dominates) and `ca795_vibe_tdd_freeze_bind_test.go` (pack bind tdd, spawn tdd then coder, block coder without contract). Old CA-785 / topology v2 / tdd binding / compose CA-794 prompt tests run as related patterns.

## Providers

Agnostic: YAML behavior string + `freezeWriterBinding` / `resolveFreezeWriterTarget` take no `providerKey` and do not branch on one. Spawn tests use the shared freeze fake (Codex key) like existing CP-55 freeze tests; adapters are not on this path.

## Will not undo

CA-794 signature + implement prompts. CA-785 fixture: delegate-between-freeze still fail-closed / bind first `agent.code`. CA-791 join. CA-793 checkpoint exist-gate.

Follow-up (docs, not this change): SD-24 / CP-60 P-7 / Task-323 T-1 still say `tdd` is `agent.delegate`. Pack is now `agent.code`.
