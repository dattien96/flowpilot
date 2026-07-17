# CA-355 — Gate keep-test-fix-code no longer re-opens modal every turn

## Context

Live `run-11262` (coder under Review Loop hub `run-11257`): user submitted **Fix the code** three times. Each turn the model only described the bug / asked for write permission in chat; `calc.go` stayed `a + b - 156`. Post-turn r-reg still red → full regression decision modal again → infinite operator loop.

## Fix

- After `keep-test-fix-code`, arm `gateFixCodeActive` on the run.
- Subsequent r-reg blocks while active: **silent auto-reprompt** (strong write-required prompt) up to `maxGateFixCodeAutoReprompts` (2); emit status `reprompt` with **no GateOptions** so desktop does not open the radio modal.
- Budget exhausted → re-show decision card once.
- Green gate path / Stop clear the flag.
- Stronger first remediation prompt (must call write/edit tool; do not end turn only asking permission in prose).

## Verify

```bash
cd apps/local-runner
go test ./internal/runner/ -count=1 -run 'TestKeepTestFixCodePrompt|TestApplyGateFixCodeAutoReprompt'
```

Operator: rebuild runner; YOLO/write approval still required for Claude when YOLO off — approve the **write tool** card if shown, not only the regression modal.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Stop r-reg decision modal loop after keep-test-fix-code when agent fails to write (run-11262)
# --->8---
