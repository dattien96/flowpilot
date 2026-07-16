# CA-328: BUG-288 Flow-Mode Three-Tier Gate Lifecycle And Change Contract Re-entry Gaps

## Scope

Closes Codex multi-round FAIL findings on Phase A (Task-238–242 three-tier gate) and Phase B (CP-50 / Task-244–247 change.contract re-entry): child gate after premature settle, missing ContractDeclared, validate oracle/baseline gaps, re-entry prompt missing change.contract, git dirty-worktree false positives, cancel/oracle/audit races, and related lifecycle/resume correctness (through Vòng 9–12).

## Changes

- `apps/local-runner/internal/runner/`: defer flow-engine child settle until gate pass; parent-run contract capture; re-entry/reprompt `appendChangeContractIfAny`; validate oracle path; cancel ≠ advance; gate reprompt keeps deps unsatisfied; Gemini git guard; source-excerpt open helpers; step-transition / workflow-store durability; resume and interactive lifecycle hardening.
- `apps/local-runner/internal/flowgate/`: baseline/observe/oracle/rules fixes for monorepo baseline, per-turn dirty fingerprint, oracle stream caps.
- Tests: `bug288_*`, `v9_matrix`, `v10_residual`, gate/context/oracle coverage.
- Skillpack: `codex-claude-review-loop` / `codex-grok-review-loop` flow-pack entries.
- Doc: `requirements/09-BugFix/inprogress/BUG-288-...md` (status remains inprogress pending fresh Codex re-review pass).

## Verification

- Unit/integration coverage added for V9/V10/R11/R12 residual matrix and gate/contract paths (runner + flowgate packages).
- Status of BUG-288 kept `inprogress` until a clean Codex re-review confirms no remaining blockers.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-288
change_type: bugfix
summary: fix three-tier gate lifecycle (defer settle, contract on parent run, re-entry change.contract inject, validate oracle, cancel/git/resume races) and residual Codex findings through Vòng 12
# --->8---
