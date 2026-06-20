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

  // Find canonical coder/reviewer runs for side-by-side SVG rendering
  const coderRun = useMemo(() => {
    return snapshot?.runs.find((run) => run.agentName.toLowerCase().includes("coder"));
  }, [snapshot]);

  const reviewerRun = useMemo(() => {
    return snapshot?.runs.find((run) => run.agentName.toLowerCase().includes("reviewer"));
  }, [snapshot]);

  const loopStatus = snapshot?.loopState.status ?? "idle";
  const loopRound = snapshot?.loopState.round ?? 0;
  const loopRoundCap = snapshot?.loopState.roundCap ?? 3;
  const loopGate = snapshot?.loopState.gateReason;

  return (
    <section className="board">
      <header className="board-head">
        <div>
          <h2>▦ Orchestration</h2>
          <span className="board-goal">
            Goal: <b>Land BUG-094 fix, reviewer-approved</b>
          </span>
        </div>
        {snapshot?.loopState && (
          <span className="status agents font-semibold">
            <span className="pulse" />
            round {loopRound} / {loopRoundCap}
          </span>
        )}
      </header>

      {!canRender ? (
        <div className="board-empty" style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center", minHeight: "200px" }}>
          {emptyCopy}
        </div>
      ) : (
        <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: "16px" }}>
          {/* Main Nodes Diagram */}
          {coderRun && reviewerRun ? (
            <div className="bgrid">
              {/* Coder Node */}
              <div className="node coder">
                <h3>
                  <span className={`sd ${coderRun.status === "running" ? "run" : (coderRun.status === "waiting_approval" || coderRun.status === "waiting_question") ? "wait" : "done"}`} />
                  {coderRun.agentName}
                </h3>
                <div className="role">Claude · implements the change</div>
                <div className="state">
                  <span className={`sd ${coderRun.status === "running" ? "run" : "done"}`} />
                  {coderRun.status === "running" ? "running — applying feedback" : coderRun.status}
                </div>
                {coderRun.agentStatus && (
                  <div className="substep">
                    ▸ {coderRun.agentStatus}
                  </div>
                )}
                <button type="button" className="secondary-btn btn-sm" onClick={() => void focusAgentRun(coderRun.runId)}>
                  Focus chat
                </button>
              </div>

              {/* Edge SVG Connector */}
              <div className="edge">
                <svg viewBox="0 0 70 110" preserveAspectRatio="none">
                  <defs>
                    <marker id="g" markerWidth="9" markerHeight="9" refX="7" refY="3" orient="auto">
                      <path d="M0,0 L7,3 L0,6 Z" fill="#3fb950" />
                    </marker>
                    <marker id="v" markerWidth="9" markerHeight="9" refX="7" refY="3" orient="auto">
                      <path d="M0,0 L7,3 L0,6 Z" fill="#b07cff" />
                    </marker>
                  </defs>
                  <path d="M2,34 L66,34" fill="none" stroke="#3fb950" strokeWidth="2" markerEnd="url(#g)" />
                  <path d="M66,76 L4,76" fill="none" stroke="#b07cff" strokeWidth="2" markerEnd="url(#v)" />
                </svg>
                <span className="lbl" style={{ top: "14px" }}>diff ▸</span>
                <span className="lbl" style={{ top: "64px" }}>◂ feedback</span>
              </div>

              {/* Reviewer Node */}
              <div className="node reviewer">
                <h3>
                  <span className={`sd ${reviewerRun.status === "running" ? "run" : (reviewerRun.status === "waiting_approval" || reviewerRun.status === "waiting_question") ? "wait" : "done"}`} />
                  {reviewerRun.agentName}
                </h3>
                <div className="role">Codex · gates the change</div>
                <div className="state">
                  <span className={`sd ${reviewerRun.status === "running" ? "run" : "done"}`} />
                  {reviewerRun.status === "running" ? "running — evaluating diff" : reviewerRun.status}
                </div>
                {loopGate ? (
                  <div className="substep">
                    round {loopRound - 1} verdict: <span style={{ color: "var(--warn)", fontWeight: "bold" }}>{loopGate}</span>
                  </div>
                ) : reviewerRun.agentStatus ? (
                  <div className="substep">
                    ▸ {reviewerRun.agentStatus}
                  </div>
                ) : null}
                <button type="button" className="secondary-btn btn-sm" onClick={() => void focusAgentRun(reviewerRun.runId)}>
                  Focus chat
                </button>
              </div>
            </div>
          ) : (
            <div className="board-graph" role="list" aria-label="Agent dependency graph">
              {snapshot!.runs.map((run) => {
                const focused = run.runId === activeAgentRunId;
                const statusDotColor = run.status === "running" ? "var(--ok)" : (run.status === "waiting_approval" || run.status === "waiting_question") ? "var(--warn)" : "var(--accent)";
                return (
                  <article key={run.runId} className={`node ${focused ? "coder" : "reviewer"}`} role="listitem" style={{ borderTopColor: statusDotColor }}>
                    <div className="board-node-top">
                      <strong>{run.agentName || run.runId}</strong>
                      <span>{run.status}</span>
                    </div>
                    <div className="role">{run.role || "agent"}</div>
                    {run.agentStatus && <div className="substep">▸ {run.agentStatus}</div>}
                    <div className="board-node-actions">
                      <button type="button" className="secondary-btn btn-sm" onClick={() => void focusAgentRun(run.runId)}>Focus</button>
                    </div>
                  </article>
                );
              })}
            </div>
          )}

          {/* Iteration Summary Banner */}
          <div className="board-round">
            Iteration <b>{loopRound} of {loopRoundCap}</b> · auto-approve when reviewer returns <b style={{ color: "var(--ok)" }}>APPROVED</b> → handoff ▸ main agent commits
          </div>

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
