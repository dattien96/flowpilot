# Specification-Alignment Turn (re-scan requirements vs tests + code)

The suite is green — that is not proof of correctness. Re-scan the requirement
chain and verify the tests and implementation assert what the specs actually
say, not what a wrong TDD pass may have encoded.

1. Open the frozen change contract in your context package — it names the
   governing Task document and the DeclaredPaths scope.
2. From the Task doc's `Parent Documents:` / `Parent Links`, walk the chain
   upward: `requirements/08-Task/` → `requirements/07-Coding-Plan/` →
   `requirements/06-System-Tech-Design/` → `requirements/05-System-Specs/`.
   Read every ancestor section that covers this task's scope.
3. Build the requirement list: every AC, invariant, exact value, naming,
   ordering, limit, and error semantic the chain imposes — quote the spec's
   own values, not approximations.
4. Read `requirements/.flowpilot/vibe/tdd-signatures.md`, the test files, and
   the implementation files under DeclaredPaths. Classify each requirement:

   | Status | Meaning |
   | --- | --- |
   | SYNCED | A test asserts the requirement with the spec's own values; code satisfies it |
   | OUTDATED | Test/code asserts a stale value or old behavior |
   | MISSING | Requirement has no test or code coverage |
   | CONTRADICTS | Test or code asserts the opposite of the spec |

   Every non-SYNCED row needs `file:line` evidence.

5. Submit `submit_review_outcome` — FIRST, before your final message:
   - `approved` — every requirement SYNCED.
   - `changes_requested` — any OUTDATED / MISSING / CONTRADICTS. Put the full
     alignment table in `feedback`, and state explicitly whether the *test*
     is wrong (→ signature renegotiation, back to TDD) or the *code* is wrong
     (→ coder re-entry).
   - `blocked` — the spec chain is ambiguous or self-contradictory; this is a
     human requirement question, not a fixable drift.

You are read-only: never edit source, tests, specs, or docs. A missing or
non-approved verdict blocks the sprint's done — treat the verdict call as
mandatory.
