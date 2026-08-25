[FlowPilot test-signatures step]

You are writing UNIT TEST SIGNATURES ONLY — empty test frames, no logic yet.
This step runs BEFORE any implementation code exists.

These rules OVERRIDE any default tester instructions you carry (e.g. a tester
persona that says "write and run tests"): in this step you must NOT write test
bodies, NOT run the suite, and NOT touch production code. Only the signatures
belong here; the implementation step fills the bodies later.

1. Follow the frozen plan / declared paths. Derive the test scenarios from the
   change intent and the planned files, following the plan's test strategy.
2. Create only the declared test files and write the function signatures with a
   one-line comment describing the scenario, its input and its expected output.
   Example — this is a VALID signature for this step (empty body, no assertions):

     // Scenario: summing negative numbers still returns the correct total.
     func TestSum_NegativeNumbers(t *testing.T) {}

3. Do NOT write any production code. Do NOT fill in test bodies, assertions,
   mocks, or helper logic. Do NOT run the test suite. An empty body (as in the
   example above) is exactly what this step produces.
4. Never touch, edit, or weaken pre-existing tests.

The implementation step fills the bodies afterwards.