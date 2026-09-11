# CA-794 — vibe-sprint tdd reuses harness signature-only prompt

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: vibe-sprint tdd appends prompts/test-signatures.md (harness parity); coder gets implement-complete-tests.md; tdd stays agent.delegate
# --->8---

Topology claim `tdd stays agent.delegate` is **superseded by [CA-795](./CA-795-Vibe-Tdd-Agent-Code.md)**. Prompt attachments in this note still apply.

## Why

Live run-219176 (OpenCode tester, YOLO): vibe `tdd` wrote `snake/snake.go` and full `snake_test.go` bodies. `agents/tester.md` default is "Write and run tests". task-harness `test_signatures` overrides that with `prompts/test-signatures.md`. vibe-sprint `tdd` had the tester agent and `tdd_signatures` binding but **no** `promptTemplate`.

## Change

- `flows/vibe-sprint.yaml` `tdd`: `promptTemplate: prompts/test-signatures.md` (empty frames, no production, no bodies). Keep `behavior: agent.delegate` (CA-785 freeze binds coder).
- `coder`: `promptTemplate: prompts/implement-complete-tests.md` so the next node fills signatures (harness pair).
- `composeFlowNodeAgentPrompt` already appends the static template on spawn.

## Tests

New `ca794_vibe_tdd_signatures_prompt_test.go` (pack) and `ca794_vibe_tdd_signature_prompt_test.go` (compose). Old `TestPack_VibeSprintV2Topology` / CA-785 freeze tests untouched.

## Providers

Agnostic: pack YAML + `appendStaticNodePrompt` take no `providerKey`. Live miss was OpenCode; Claude/Grok would hit the same missing template.

## Will not undo

CA-785 fixture (delegate-between-freeze) unchanged at the time of this note; pack `tdd.behavior` later became `agent.code` in CA-795. CA-791 join. CA-793 checkpoint exist-gate.
