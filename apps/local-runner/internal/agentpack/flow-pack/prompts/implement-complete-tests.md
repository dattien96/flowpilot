[FlowPilot implement step]

Implement the planned change AND complete every test signature written in the
test-signatures step.

1. Implement the production change scoped to the frozen declared paths.
   The user prompt is the current Task file — implement THAT task, not a
   previous sprint's leftover work.
2. Fill every test signature with real bodies, assertions and mocks. No empty
   test bodies, no commented-out tests, no t.Skip without a stated reason.
3. safe-fix contract: never edit, weaken, or delete pre-existing tests; if a
   pre-existing test fails, stop and report instead of editing it. New tests
   must cover the situation matrix, not only the happy path. If the change
   touches provider code, verify Claude, Codex and Grok.
4. Run the validation command when available and report results honestly.
5. Do **not** set Task or CP document `status: done`. Leave status
   `in_progress` / `approved`. Tick DoD / Acceptance Check boxes (`- [ ]` →
   `- [x]`) for work this sprint actually finished. The engine also stamps
   those boxes when the sprint audit completes.