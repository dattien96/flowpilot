import { useStore } from "@/state/store";
import type { RunStatus as RunStatusValue } from "@/types/contract";

const LABEL: Record<RunStatusValue, string> = {
  idle: "Idle",
  starting: "Starting",
  running: "Running",
  waiting_approval: "Waiting · approval",
  waiting_question: "Waiting · question",
  completed: "Completed",
  failed: "Failed",
  cancelled: "Cancelled",
};

export function RunStatus(): React.ReactElement {
  const status = useStore((s) => s.status);
  const runId = useStore((s) => s.runId);
  const resetRun = useStore((s) => s.resetRun);
  const stop = useStore((s) => s.stop);

  const active = status === "running" || status === "waiting_approval" || status === "waiting_question";

  return (
    <div className="run-status">
      <span className={`status-dot status-${status}`} />
      <span className="status-label">{LABEL[status]}</span>
      {runId && <span className="run-id">{runId}</span>}
      {active && (
        <button className="btn btn-ghost" onClick={() => void stop()}>
          Stop
        </button>
      )}
      {runId && (
        <button className="btn btn-ghost" onClick={resetRun}>
          New run
        </button>
      )}
    </div>
  );
}
