version: 1

# oracle-rule

Enforce the Oracle Rule: never change a test to make it pass; fix the code instead.

## The Rule

> Tests are the source of truth about intended behavior. When a test fails, the code is wrong — not the test.

## Behavior

1. If a pre-existing test breaks after your change, **stop and diagnose the root cause in the production code**.
2. Do NOT weaken assertions, comment out tests, skip test cases, or change expected values to match broken output.
3. Do NOT delete a failing test unless the spec/AC explicitly says the requirement has been removed.
4. If you believe the test itself is incorrect (contradicts the spec), **ask the user before modifying it** and cite the spec document and section.

## When a Test Fails

- Check the acceptance criteria in the linked Task or BUG document.
- If the AC and the test agree: the production code is wrong — fix it.
- If the AC and the test disagree: surface the conflict to the user; do not resolve it silently.
- If there is no AC: treat the test as the spec; fix the code.

## Rationale

Weakening tests to pass CI hides regressions and erodes the regression suite over time. The oracle is the test; the oracle is not negotiable without a human decision.
