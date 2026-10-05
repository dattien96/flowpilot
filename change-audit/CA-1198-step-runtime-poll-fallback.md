# CA-1198 — Step rail catch-up poll (BUG-1198)

## Evidence

`desktop|step-list-and-card-stale-after-burst` (live run-225691): the step
timeline showed `tdd RUNNING` + `coder/validate/spec_align/reviewer PENDING`
at 13:15:45 — ~104s after the runner had already transitioned them
(`coder/validate/spec_align DONE`, `reviewer RUNNING` at 13:13:28–13:14:01).
A resolved decision card also kept rendering.

`refreshWorkflowStepRuntime` is purely event-driven (`agent_graph_updated`
SSE → fetch). A dropped/missed event burst leaves the list stale forever —
no fallback existed.

## Fix

`stepRuntimeNeedsPoll` (components/flowStepRuntimePoll.ts) decides whether a
slow 10s fallback interval runs: live run + empty list (mount-fetch race) or
any non-terminal row → poll; all-terminal → stop. `FlowTimelineSidebar`
mounts the interval on that predicate; event refreshes remain primary.

## Tests

`flowStepRuntimePoll.test.ts` (node:test): non-terminal polls, all-terminal
stops, empty-list-on-run polls, no-run never polls. `tsc --noEmit` clean.
