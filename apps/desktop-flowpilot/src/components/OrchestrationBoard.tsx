import { useEffect, useMemo, useState } from "react";
import type { AgentBusMessage, AgentGraphSnapshot, AgentRunSummary } from "@/types/contract";
import { useStore } from "@/state/store";

export function getOrchestrationBoardEmptyCopy(snapshot?: AgentGraphSnapshot, agentRuns: AgentRunSummary[] = []): string {
  if (!snapshot) return "No graph snapshot loaded. Refresh to inspect the loop.";
  if (snapshot.runs.length === 0 && agentRuns.length === 0) return "No child agents yet.";
  if (snapshot.runs.length === 0) return "Snapshot loaded, but no child agents are currently tracked.";
  return "";
}

export function getBusMessageLabel(message: AgentBusMessage): string {
  return message.queued ? `${message.kind}: ${message.message} (queued)` : `${message.kind}: ${message.message}`;
}

export function OrchestrationBoard(): React.ReactElement {
  const snapshot = useStore((s) => s.agentGraphSnapshot);
  const agentRuns = useStore((s) => s.agentRuns);
  const activeAgentRunId = useStore((s) => s.activeAgentRunId);
  const refresh = useStore((s) => s.refreshAgentGraph);
  const refreshRuns = useStore((s) => s.refreshAgentRuns);
  const pause = useStore((s) => s.pauseAgentLoop);
  const resume = useStore((s) => s.resumeAgentLoop);
  const stop = useStore((s) => s.stopAgentLoop);
  const focusAgentRun = useStore((s) => s.focusAgentRun);
  const injectAgentFeedback = useStore((s) => s.injectAgentFeedback);
  const openAgentSpawnGuide = useStore((s) => s.openAgentSpawnGuide);
  const mainRunId = useStore((s) => s.mainRunId ?? s.runId);

  const [feedback, setFeedback] = useState("");
  const focusTarget = useMemo(() => snapshot?.runs.find((run) => run.status === "running")?.runId ?? activeAgentRunId, [snapshot, activeAgentRunId]);

  useEffect(() => {
    void refresh();
    void refreshRuns();
  }, [mainRunId, refresh, refreshRuns]);

  const emptyCopy = getOrchestrationBoardEmptyCopy(snapshot, agentRuns);
  const canRender = Boolean(snapshot) && emptyCopy.length === 0;
  return (
    <section className="orchestration-board">
      <header className="board-head">
        <div>
          <div className="board-kicker">Orchestration</div>
          <h3>Dependency graph</h3>
          <p>
            {snapshot ? `${snapshot.loopState.status} · round ${snapshot.loopState.round}/${snapshot.loopState.roundCap}` : "Refresh to inspect the loop."}
          </p>
          {snapshot?.loopState.gateReason && <div className="board-gate">Gate: {snapshot.loopState.gateReason}</div>}
        </div>
        <div className="board-actions">
          <button type="button" onClick={() => void refresh()}>Refresh graph</button>
          <button type="button" onClick={() => void refreshRuns()}>Refresh runs</button>
          <button type="button" onClick={() => void pause()}>Pause</button>
          <button type="button" onClick={() => void resume()}>Resume</button>
          <button type="button" onClick={() => void stop()}>Stop</button>
          <button type="button" onClick={() => openAgentSpawnGuide("coder")}>Add agent</button>
        </div>
      </header>

      <div className="board-compose">
        <input value={feedback} onChange={(e) => setFeedback(e.target.value)} placeholder="Inject feedback to the active child..." />
        <button type="button" disabled={!focusTarget || feedback.trim().length === 0} onClick={() => focusTarget && void injectAgentFeedback(focusTarget, feedback).then(() => setFeedback(""))}>
          Inject feedback
        </button>
      </div>

      {canRender ? (
        <>
          <div className="board-graph" role="list" aria-label="Agent dependency graph">
            {snapshot!.runs.map((run) => {
              const focused = run.runId === activeAgentRunId;
              return (
                <article key={run.runId} className={`board-node ${focused ? "active" : ""}`} role="listitem">
                  <div className="board-node-top">
                    <strong>{run.agentName || run.runId}</strong>
                    <span>{run.status}</span>
                  </div>
                  <div className="board-node-role">{run.role || "agent"}</div>
                  <div className="board-node-meta">{run.createdAt}</div>
                  <div className="board-node-actions">
                    <button type="button" onClick={() => void focusAgentRun(run.runId)}>Focus</button>
                  </div>
                </article>
              );
            })}
          </div>

          <div className="board-edges">
            {snapshot!.edges.length === 0 ? <p>No dependency edges yet.</p> : snapshot!.edges.map((edge, idx) => <div key={`${edge.fromRunId}-${edge.toRunId}-${idx}`}>{edge.kind}: {edge.fromRunId} → {edge.toRunId}</div>)}
          </div>

          <div className="board-bus">
            <h4>Bus log</h4>
            {snapshot!.busMessages.length === 0 ? <p>No bus messages yet.</p> : snapshot!.busMessages.map((msg) => <div key={msg.id} className={`bus-pill ${msg.queued ? "queued" : ""}`}>{getBusMessageLabel(msg)}</div>)}
          </div>

          <div className="board-foot">
            <span>Children: {agentRuns.length}</span>
            <span>Focus: {focusTarget ?? "none"}</span>
          </div>
        </>
      ) : (
        <div className="board-empty">{emptyCopy}</div>
      )}
    </section>
  );
}
