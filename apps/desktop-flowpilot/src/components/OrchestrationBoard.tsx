import { useEffect, useMemo, useState } from "react";
import type { AgentBusMessage, AgentGraphSnapshot, AgentRunSummary } from "@/types/contract";
import { useStore } from "@/state/store";

function nodeStatusClass(status: string): string {
  if (status === "running") return "run";
  if (status === "waiting_approval" || status === "waiting_question") return "wait";
  return "done";
}

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
  const submitReviewOutcome = useStore((s) => s.submitReviewOutcome);
  const extendCap = useStore((s) => s.extendCap);
  const mainRunId = useStore((s) => s.mainRunId ?? s.runId);

  const [feedback, setFeedback] = useState("");
  const focusTarget = useMemo(() => snapshot?.runs.find((run) => run.status === "running")?.runId ?? activeAgentRunId, [snapshot, activeAgentRunId]);

  useEffect(() => {
    void refresh();
    void refreshRuns();
  }, [mainRunId, refresh, refreshRuns]);

  const emptyCopy = getOrchestrationBoardEmptyCopy(snapshot, agentRuns);
  const canRender = Boolean(snapshot) && emptyCopy.length === 0;

  const loopState = snapshot?.loopState;
  const loopStatus = loopState?.status ?? "idle";
  const loopRound = loopState?.round ?? 0;
  const loopCap = loopState?.cap ?? loopState?.roundCap ?? 3;
  const loopGate = loopState?.gateReason;
  const openIssues = loopState?.openIssues;
  const isBlocked = loopStatus === "blocked";

  return (
    <section className="board">
      <header className="board-head">
        <div>
          <h2>▦ Orchestration</h2>
          {loopState?.activeNode && (
            <span className="board-goal">
              Active: <b>{loopState.activeNode}</b>
            </span>
          )}
        </div>
        {loopState && (
          <span className={`status agents font-semibold${isBlocked ? " warn" : ""}`}>
            <span className="pulse" />
            round {loopRound} / {loopCap}
            {openIssues !== undefined && openIssues > 0 && (
              <span style={{ marginLeft: "8px", color: "var(--warn)" }}>{openIssues} open</span>
            )}
            {loopState.mode && (
              <span style={{ marginLeft: "8px", opacity: 0.6 }}>{loopState.mode}</span>
            )}
          </span>
        )}
      </header>

      {!canRender ? (
        <div className="board-empty" style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center", minHeight: "200px" }}>
          {emptyCopy}
        </div>
      ) : (
        <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: "16px" }}>
          {/* N-child nodes row */}
          <div className="board-graph" role="list" aria-label="Agent dependency graph">
            {snapshot!.runs.map((run) => {
              const focused = run.runId === activeAgentRunId;
              const sc = nodeStatusClass(run.status);
              const statusDotColor = sc === "run" ? "var(--ok)" : sc === "wait" ? "var(--warn)" : "var(--accent)";
              return (
                <article key={run.runId} className={`node ${focused ? "coder" : "reviewer"}`} role="listitem" style={{ borderTopColor: statusDotColor }}>
                  <div className="board-node-top">
                    <h3 style={{ margin: 0 }}>
                      <span className={`sd ${sc}`} />
                      {run.agentName || run.runId}
                    </h3>
                    <span style={{ fontSize: "0.75rem", opacity: 0.7 }}>{run.status}</span>
                  </div>
                  {run.role && run.role !== run.agentName && <div className="role">{run.role}</div>}
                  {run.agentStatus && <div className="substep">▸ {run.agentStatus}</div>}
                  {loopGate && run.role?.toLowerCase().includes("reviewer") && (
                    <div className="substep">
                      verdict: <span style={{ color: "var(--warn)", fontWeight: "bold" }}>{loopGate}</span>
                    </div>
                  )}
                  <div className="board-node-actions">
                    <button type="button" className="secondary-btn btn-sm" onClick={() => void focusAgentRun(run.runId)}>Focus</button>
                  </div>
                </article>
              );
            })}
          </div>

          {/* Iteration summary */}
          <div className="board-round">
            Round <b>{loopRound} / {loopCap}</b>
            {openIssues !== undefined && <span> · <b style={{ color: openIssues > 0 ? "var(--warn)" : "var(--ok)" }}>{openIssues} open issue{openIssues !== 1 ? "s" : ""}</b></span>}
            <span style={{ marginLeft: "8px", opacity: 0.7 }}>status: {loopStatus}</span>
          </div>

          {/* Blocked banner + Extend-cap control */}
          {isBlocked && (
            <div className="board-blocked" style={{ padding: "10px 12px", background: "var(--warn-bg, rgba(255,180,0,0.12))", borderRadius: "6px", display: "flex", gap: "8px", alignItems: "center", flexWrap: "wrap" }}>
              <span style={{ color: "var(--warn)", fontWeight: "bold" }}>⚠ Blocked</span>
              {loopGate && <span style={{ opacity: 0.8 }}>{loopGate}</span>}
              <button type="button" className="bc warn" onClick={() => void extendCap()}>Extend cap +2</button>
              <button type="button" className="bc primary" onClick={() => void submitReviewOutcome("approved")}>Accept &amp; approve</button>
            </div>
          )}

          {/* Action Controls */}
          <div className="bctl">
            <button type="button" className="bc primary" onClick={() => void resume()}>▸ Resume loop</button>
            <button type="button" className="bc" onClick={() => void pause()}>⏸ Pause</button>
            <button type="button" className="bc warn" onClick={() => openAgentSpawnGuide("tester")}>⤵ Add tester</button>
            <button type="button" className="bc err" onClick={() => void stop()}>⨉ Stop all</button>
          </div>

          {/* Inject Feedback Row */}
          <div className="board-compose">
            <input
              type="text"
              value={feedback}
              onChange={(e) => setFeedback(e.target.value)}
              placeholder="Inject feedback to the active child..."
            />
            <button
              type="button"
              className="primary-btn"
              disabled={!focusTarget || feedback.trim().length === 0}
              onClick={() => focusTarget && void injectAgentFeedback(focusTarget, feedback).then(() => setFeedback(""))}
            >
              Inject feedback
            </button>
          </div>

          {/* Live Bus Log */}
          <div className="log">
            <div className="lh">Agent bus · live</div>
            <div className="log-body">
              {snapshot!.busMessages.length === 0 ? (
                <div className="ln" style={{ color: "var(--text-dim)", fontStyle: "italic" }}>No bus messages yet.</div>
              ) : (
                snapshot!.busMessages.map((msg) => {
                  const time = new Date(msg.occurredAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
                  const lowerSender = msg.fromRunId ? (snapshot!.runs.find(r => r.runId === msg.fromRunId)?.agentName.toLowerCase() ?? "") : "orchestrator";
                  const senderClass = lowerSender.includes("coder") ? "a-coder" : lowerSender.includes("reviewer") ? "a-rev" : "a-orc";
                  const senderName = msg.fromRunId ? (snapshot!.runs.find(r => r.runId === msg.fromRunId)?.agentName ?? msg.fromRunId) : "orchestrator";

                  return (
                    <div key={msg.id} className="ln">
                      <span className="t">{time}</span>
                      <span className={senderClass}>{senderName}</span>
                      <span style={{ color: "var(--text)" }}>→ bus: {msg.message}</span>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
