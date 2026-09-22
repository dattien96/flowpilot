---
name: flow-harness-contract
description: Harness discipline for every coding flow — frozen scope, reproduce-first bugfix, contract-first TDD with signature lock, machine-checkable done verdicts, doc intent ceiling, drift self-correction, and session resume checkpoints. Use PROACTIVELY on every task, bugfix, or feature implementation.
version: 6
---

# flow-harness-contract

Umbrella rule card for executing work the way a gated engineering flow does it,
even when no tool is enforcing the gates. Companion to **safe-fix-contract**
(test safety), **context-discipline** (feature history + change contract),
**phase-doc** (document contract), **audit-logging** (ledger). Where rules
overlap, the stricter one wins.

---

## 1. Scope: declare → freeze → amend

1. Before any code-mutating edit, emit the change-contract block (see
   `context-discipline`): `feature_key`, one-line intent, and the file list you
   expect to touch.
2. Once implementation starts, the declared file list is **frozen**. An edit
   outside it is drift — stop, restate the contract with the expanded scope
   (`files: + <new paths>`), then continue. Never silently widen scope.
3. A contract inferred *after* edits (from the diff) is not a preflight
   contract. Declare first; the diff validates the declaration, it does not
   create it.
4. The canonical truth for a feature is its **latest ledger entry + governing
   spec doc** — not the raw commit churn. Build on it; never re-implement or
   silently undo prior settled work.

## 2. Bugfix = reproduce-first

1. Before touching production code for a bug, write **one complete failing
   test** that reproduces it.
2. The test must fail by **assertion failure** (`expected X, got Y`). A compile
   error or panic does not count as reproduction — fix the test until it
   compiles and fails on the assertion.
3. Once the red test is confirmed, that test file is **locked**: fix production
   code only. Never reshape the reproducing test to fit your fix.
4. If no failing test can be produced, state that explicitly and ask the user —
   do not proceed on a guessed fix.

## 3. New feature = contract-first TDD

1. Before implementation: produce compile-clean **stubs** (bodies =
   `not implemented` / zero-sentinels) plus a **complete test suite** that
   compiles and runs RED at runtime.
2. Once the suite exists, all public signatures are **locked**. Implementation
   fills bodies only — never renames, re-types, or reshapes a signature.
3. If a signature proves inadequate mid-implementation: do not edit it inline.
   Accumulate the needed changes and raise **one batched request** at end of
   turn — to the user, or the orchestrating agent — for renegotiation. The
   implementer never unilaterally rewrites the contract it was handed.
4. Negotiation is hub-and-spoke, never peer-to-peer between implementer and
   test author.

## 4. "Done" requires a machine-checkable verdict

1. Never declare done in prose. A done claim needs a **verdict**: each
   acceptance criterion → pass/fail + evidence (test name, `file:line`, command
   output).
2. Review/fix loops are **bounded** (default 3 rounds). At the cap with open
   findings: escalate to the user with options (extend / accept / stop). Never
   silently stop, never retry unbounded.
3. A green suite reached by weakening, skipping, or tampering with tests is a
   violation — not a pass (see `safe-fix-contract` R1).
4. A check that cannot run (no baseline, broken env, missing tool) must be
   reported as **blind/unverified** — never reported as pass. Fail closed.

## 5. Documents: hard ceiling on intent

1. Code shows **what** the system does; it can never show **why** the business
   wants it. Never fabricate business intent, user stories, or acceptance
   criteria in spec-level documents — scaffold the skeleton and mark
   `TODO: human intent needed`.
2. Spec/requirement-level content becomes authoritative **only after explicit
   human confirmation** (a lock/preview step). Do not derive downstream plans
   from unconfirmed specs.
3. Every `Task-*`/`BUG-*` document must carry a `## Definition of Done`
   section with checkboxes. Marking a doc done with unchecked items requires an
   explicit written justification — silence is a violation.
4. Never tick a checkbox on behalf of anyone; `[x]` is a deliberate act.

## 6. Drift: self-correction ladder

Watch for these signals in your own work:

| Signal | Meaning |
|---|---|
| Same test failing ≥ 2 times | You are guessing, not diagnosing |
| Repeated apology/retry loops | Wrong approach, not wrong attempt |
| Turn produced no code delta | Stall — re-plan, don't re-run |
| Edits drifting outside declared scope | Scope breach — amend or revert |

Ladder, in order — **never skip to a harsher step, never hard-rollback**:

1. Stop; restate the goal and the frozen contract in one line each.
2. Narrow context to the failing seam (one file, one function, one test).
3. Still stuck → stop and ask the user one concrete question.

## 7. Mistakes become lessons — only with approval

1. A **repeated** mistake (same failure class twice) → draft a lesson card:
   symptom, root cause, one-line rule.
2. Propose the card to the user. Only a **human-approved** lesson becomes a
   durable rule or skill file.
3. Never auto-create permanent rule/skill docs from a single incident — that
   produces knowledge garbage.

## 8. Session checkpoint & resume

1. On any non-trivial flow, maintain a `STATE.md` (or equivalent checkpoint
   file) in the workspace: current phase, what's done, what's next, the frozen
   contract, open findings.
2. Update it at every pause point, gate, and before ending a session.
3. A fresh session must **read the checkpoint first** before doing anything —
   resume from it, never reconstruct state from memory or chat scroll.
4. If the checkpoint and the working tree disagree, trust the tree and say so;
   the checkpoint is a guide, not a rewrite license.
