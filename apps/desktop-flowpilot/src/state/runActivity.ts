import type { ProviderEventDTO, RunStatus } from "@/types/contract";

// Task-458: run-level liveness derived from SSE ARRIVAL timestamps. The
// backend already emits enough signal (message_delta, tool_started,
// token_usage_updated, agent_graph_updated, turn events) — stamping each
// arrival gives the UI an honest "is this run still producing events"
// indicator with zero runner changes. Stamps are keyed by the run the event
// belongs to so orchestration events land on the right lane.

/** Silence window after which a still-running step reads "quiet" (AC-2). */
export const RUN_ACTIVITY_STALE_MS = 30_000;

export type RunActivityKind = "live" | "quiet";

/** Run id an event is about. Mirrors isEventForRun's routing in store.ts —
 *  graph/bus events carry their identity on the payload, not workflowRunId. */
export function eventActivityRunId(e: ProviderEventDTO): string {
  if (e.type === "agent_graph_updated") return e.agentGraphSnapshot.parentRunId;
  if (e.type === "agent_bus_message") return e.agentBusMessage.parentRunId;
  return e.workflowRunId;
}

/** BUG-584: agent_activity arrives on the parent's stream but reports a
 *  leg's liveness — credit BOTH lanes (leg id + parent workflowRunId) so the
 *  family chip warms even if the leg's agentRuns row is momentarily stale. */
function eventActivityRunIds(e: ProviderEventDTO): string[] {
  if (e.type === "agent_activity") {
    const ids = [e.childRunId, e.workflowRunId].filter((v): v is string => Boolean(v));
    return [...new Set(ids)];
  }
  const id = eventActivityRunId(e);
  return id ? [id] : [];
}

/** Next lastActivityByRun after one arrival. Returns the same ref when the
 *  event carries no run identity so zustand subscribers do not re-render on
 *  a no-op stamp. */
export function stampRunActivity(
  byRun: Record<string, number>,
  e: ProviderEventDTO,
  at: number,
): Record<string, number> {
  const ids = eventActivityRunIds(e);
  if (ids.length === 0) return byRun;
  const next = { ...byRun };
  for (const id of ids) next[id] = at;
  return next;
}

/** Runs whose stamps count toward a flow's liveness: terminal lanes are
 *  excluded so a settled child cannot make a stuck flow look alive — and a
 *  stale stamp on a settled run is never surfaced (AC-3). */
export function activityCountsForStatus(status: RunStatus): boolean {
  return status !== "completed" && status !== "failed" && status !== "cancelled";
}

/** Most recent stamp across the given run ids (main + its non-terminal legs). */
export function lastActivityAt(
  byRun: Record<string, number>,
  runIds: readonly string[],
): number | undefined {
  let last: number | undefined;
  for (const id of runIds) {
    const at = byRun[id];
    if (at !== undefined && (last === undefined || at > last)) last = at;
  }
  return last;
}

export function runActivityKind(lastAt: number, now: number): RunActivityKind {
  return now - lastAt < RUN_ACTIVITY_STALE_MS ? "live" : "quiet";
}

/** Compact age: "4s" / "3m" / "2h". Deliberately coarse — this is a freshness
 *  hint, not a clock. Callers add "ago" where the grammar needs it. */
export function formatActivityAge(ageMs: number): string {
  const s = Math.max(0, Math.floor(ageMs / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  return `${Math.floor(m / 60)}h`;
}
