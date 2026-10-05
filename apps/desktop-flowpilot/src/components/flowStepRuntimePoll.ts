import type { WorkflowStepRuntimeDTO } from "@/types/contract";

// BUG-1198 (live run-225691): refreshWorkflowStepRuntime is event-driven —
// agent_graph_updated bursts can drop (SSE backlog, focused-agent replay),
// leaving the step rail stale indefinitely: runner truth was
// coder/validate/spec_align DONE while the rail showed them PENDING for
// ~104s, and a resolved decision card kept rendering. Event refreshes stay
// primary; this interval is the catch-up fallback while the rail is live.
export const STEP_RUNTIME_POLL_MS = 10_000;

const TERMINAL_STEP_STATUSES: ReadonlySet<WorkflowStepRuntimeDTO["status"]> = new Set([
  "DONE",
  "FAILED",
  "SKIPPED",
  "CANCELED",
]);

// stepRuntimeNeedsPoll decides whether the fallback interval should keep
// running. Poll while a flow exists and either the step list is still empty
// (the mount fetch can race ahead of the steps table) or any row is
// non-terminal — RUNNING work, PENDING successors, and WAITING_USER_APPROVAL
// parks can all drift after a missed event. An all-terminal list is settled:
// nothing left to catch up, so the poll stops.
export function stepRuntimeNeedsPoll(
  steps: readonly Pick<WorkflowStepRuntimeDTO, "status">[],
  hasRun: boolean,
): boolean {
  if (!hasRun) return false;
  if (steps.length === 0) return true;
  return steps.some((s) => !TERMINAL_STEP_STATUSES.has(s.status));
}
