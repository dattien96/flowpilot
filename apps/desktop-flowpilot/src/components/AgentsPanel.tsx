import { useEffect, useMemo, useState } from "react";
import { useStore } from "@/state/store";
import type { AgentDefinition } from "@/types/contract";
import { formatDependencyLabels } from "@/components/agentDependencies";

interface SpawnDialogState {
  agentName: string;
  prompt: string;
  providerOverride?: string;
  dependsOn?: string[];
  wait?: boolean;
}

export function AgentsPanel(): React.ReactElement {
  const client = useStore((s) => s.client);
  const projects = useStore((s) => s.projects);
  const selectedProjectId = useStore((s) => s.selectedProjectId);
  const mainRunId = useStore((s) => s.mainRunId ?? s.runId);
  const activeStatus = useStore((s) => s.status);
  const mainSnapshotStatus = useStore((s) => (mainRunId ? s._runSnapshots[mainRunId]?.status : undefined));
  const activeAgentRunId = useStore((s) => s.activeAgentRunId);
  const agentRuns = useStore((s) => s.agentRuns);
  const agentSpawnGuideOpen = useStore((s) => s.agentSpawnGuideOpen);
  const agentSpawnGuideAgentName = useStore((s) => s.agentSpawnGuideAgentName);
  const refreshAgentRuns = useStore((s) => s.refreshAgentRuns);
  const focusAgentRun = useStore((s) => s.focusAgentRun);
  const backToMainRun = useStore((s) => s.backToMainRun);
  const listAgents = useStore((s) => s.listAgents);
  const clearAgentSpawnGuide = useStore((s) => s.clearAgentSpawnGuide);
  const appendSystemMessage = useStore((s) => s.appendSystemMessage);

  const workspaceMainView = useStore((s) => s.workspaceMainView);
  const openOrchestrationBoard = useStore((s) => s.openOrchestrationBoard);
  const closeOrchestrationBoard = useStore((s) => s.closeOrchestrationBoard);

  const [open, setOpen] = useState(false);
  const [agents, setAgents] = useState<AgentDefinition[]>([]);
  const [loadingAgents, setLoadingAgents] = useState(false);
  const [dialog, setDialog] = useState<SpawnDialogState | null>(null);
  const [spawning, setSpawning] = useState(false);

  useEffect(() => {
    if (!agentSpawnGuideOpen) return;
    setOpen(true);
    setDialog({ agentName: agentSpawnGuideAgentName ?? "coder", prompt: "", wait: false, dependsOn: [] });
    clearAgentSpawnGuide();
  }, [agentSpawnGuideAgentName, agentSpawnGuideOpen, clearAgentSpawnGuide]);

  const cwd = useMemo(
    () => projects.find((project) => project.id === selectedProjectId)?.path,
    [projects, selectedProjectId],
  );

  useEffect(() => {
    if (!open) return;
    setLoadingAgents(true);
    void listAgents(cwd)
      .then((result) => {
        setAgents(result);
        setDialog((current) => current ?? (result[0] ? { agentName: result[0].name, prompt: "", wait: false, dependsOn: [] } : null));
      })
      .finally(() => setLoadingAgents(false));
  }, [cwd, listAgents, open]);

  useEffect(() => {
    void refreshAgentRuns();
  }, [refreshAgentRuns, mainRunId]);

  const spawn = async (): Promise<void> => {
    if (!dialog || !mainRunId || !client.spawnAgent) return;
    const request = {
      parentRunId: mainRunId,
      agent: dialog.agentName,
      prompt: dialog.prompt,
      provider: dialog.providerOverride,
      dependsOn: dialog.dependsOn,
      wait: dialog.wait,
    };
    setSpawning(true);
    // Close the dialog as soon as the spawn is dispatched: the child streams into the
    // Agents panel regardless of the wait toggle, and a wait:true request blocks the HTTP
    // call until the child finishes — previously that froze the dialog open the whole time
    // (and a spawn error left it open silently). (BUG-120)
    setDialog(null);
    setOpen(false);
    try {
      await client.spawnAgent(request);
      await refreshAgentRuns();
    } catch (err) {
      appendSystemMessage(`Spawn agent failed: ${err instanceof Error ? err.message : String(err)}`, "error");
    } finally {
      setSpawning(false);
    }
  };

  const mainCardActive = !activeAgentRunId || activeAgentRunId === mainRunId;
  const mainRunStatus = activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId ? mainSnapshotStatus ?? activeStatus : activeStatus;
  const mainCardBusy = mainRunStatus === "running" || mainRunStatus === "waiting_approval" || mainRunStatus === "waiting_question";

  const runningAgentCount = agentRuns.filter(
    (run) => run.status === "running" || run.status === "waiting_approval" || run.status === "waiting_question",
  ).length;

  const activeRuns = agentRuns.filter((r) => r.status !== "completed" && r.status !== "failed" && r.status !== "cancelled");
  const closedRuns = agentRuns.filter((r) => r.status === "completed" || r.status === "failed" || r.status === "cancelled");

  const dependencyCandidates = activeRuns;

  return (
    <section className="panel agents">
      <div className="panel-h">
        <span>Agents</span>
        {runningAgentCount > 0 && <span className="acount">{runningAgentCount} running</span>}
        <button
          type="button"
          className={`board-link-btn ${workspaceMainView === "board" ? "active" : ""}`}
          onClick={() => {
            if (workspaceMainView === "board") {
              closeOrchestrationBoard();
            } else {
              openOrchestrationBoard();
            }
          }}
        >
          ▦ Board
        </button>
      </div>

      <p className="panel-sub">
        {workspaceMainView === "board"
          ? "Board is open in the main pane. Click an agent to open its chat instead."
          : "Sub-agents in this session — click to open · ＋ to spawn."}
      </p>

      <div className="ag-sec">
        ● Running
      </div>

      <div className="agent-run-list" style={{ display: "flex", flexDirection: "column", gap: "6px", padding: 0 }}>
        <div
          className={`acard main ${mainCardActive && workspaceMainView !== "board" ? "sel" : ""}`}
          onClick={() => mainRunId && void backToMainRun()}
        >
          <div className="ac-top">
            <span className="ac-nm">main</span>
            <span className="ac-st">
              <span className={`sd ${mainCardBusy ? "run" : "done"}`} />
              {mainCardBusy ? "active" : "idle"}
            </span>
          </div>
          <div className="ac-meta">
            <span className="pill-prov prov-codex">CODEX</span> orchestrator
          </div>
        </div>

        {activeRuns.map((run) => {
          const isSelected = run.runId === activeAgentRunId && workspaceMainView !== "board";
          const lowerName = run.agentName.toLowerCase();
          const roleClass = lowerName.includes("coder") ? "coder" : lowerName.includes("review") ? "reviewer" : lowerName.includes("test") ? "tester" : "";
          const statusClass = run.status === "running" ? "run" : (run.status === "waiting_approval" || run.status === "waiting_question") ? "wait" : "done";
          const provClass = run.providerKey ?? "codex";
          const providerBadge = (run.providerKey ?? "codex").toUpperCase();

          return (
            <div
              key={run.runId}
              className={`acard ${roleClass} ${isSelected ? "sel" : ""}`}
              onClick={() => void focusAgentRun(run.runId)}
            >
              <div className="ac-top">
                <span className="ac-nm">{run.agentName}</span>
                <span className="ac-st">
                  <span className={`sd ${statusClass}`} />
                  {run.status}
                </span>
              </div>
              <div className="ac-meta">
                <span className={`pill-prov prov-${provClass}`}>{providerBadge}</span>
                <span>{run.role}</span>
                {run.modelName && <span className="ac-model">{run.modelName}</span>}
              </div>
              {run.dependsOn && run.dependsOn.length > 0 && (
                <div className="ac-meta" style={{ marginTop: "3px" }}>
                  ⟂ depends: {formatDependencyLabels(run.dependsOn, agentRuns).join(", ")}
                </div>
              )}
              {run.agentStatus && (
                <div className="ac-last">
                  {run.agentStatus}
                </div>
              )}
            </div>
          );
        })}

        <button type="button" className="spawn" onClick={() => setOpen(true)} disabled={!mainRunId || !client.spawnAgent}>
          ＋ Spawn agent
        </button>
      </div>

      {closedRuns.length > 0 && (
        <>
          <div className="ag-sec">
            ◌ Recently closed
          </div>
          <div className="agent-run-list" style={{ display: "flex", flexDirection: "column", gap: "6px", padding: 0 }}>
            {closedRuns.map((run) => {
              const isSelected = run.runId === activeAgentRunId && workspaceMainView !== "board";
              const provClass = run.providerKey ?? "codex";
              const providerBadge = (run.providerKey ?? "codex").toUpperCase();

              return (
                <div
                  key={run.runId}
                  className={`acard closed ${isSelected ? "sel" : ""}`}
                  onClick={() => void focusAgentRun(run.runId)}
                >
                  <div className="ac-top">
                    <span className="ac-nm">{run.agentName}</span>
                    <span className="ac-st">
                      <span className="sd closed" />
                      {run.status}
                    </span>
                  </div>
                  <div className="ac-meta">
                    <span className={`pill-prov prov-${provClass}`}>{providerBadge}</span>
                    <span>{run.role}</span>
                    {run.modelName && <span className="ac-model">{run.modelName}</span>}
                  </div>
                </div>
              );
            })}
          </div>
        </>
      )}

      {open && (
        <div className="modal-backdrop" onClick={() => { setDialog(null); setOpen(false); }}>
          <div className="dlg" onClick={(e) => e.stopPropagation()}>
            <div className="dlg-h">
              <span>🤖 Spawn sub-agent</span>
              <span className="x" onClick={() => { setDialog(null); setOpen(false); }}>✕</span>
            </div>
            <div className="dlg-b">
              <div className="fld">
                <label>Agent <span className="src-hint">— from .claude/agents · .codex/agents · built-ins</span></label>
                <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
                  {(agents.length > 0 ? agents : [
                    { name: "coder", description: "Implements a scoped change end-to-end. Defaults to Claude.", role: "coder", source: "flowpilot" },
                    { name: "reviewer", description: "Adversarial code review; can request changes and gate the loop.", role: "reviewer", source: "flowpilot" },
                    { name: "tester", description: "Writes & runs tests, reports coverage gaps.", role: "tester", source: "built-in" }
                  ]).map((agent) => {
                    const isAgentSelected = dialog?.agentName === agent.name;
                    const lowerAgentName = agent.name.toLowerCase();
                    const cardRoleClass = lowerAgentName.includes("coder") ? "coder" : lowerAgentName.includes("review") ? "reviewer" : "tester";
                    const lowerPath = (agent.path ?? "").toLowerCase().replace(/\\/g, "/");
                    const isClaudeSource = agent.source === "claude" ||
                      (agent.source === "provider" && lowerPath.includes(".claude"));
                    const isProviderAgnostic = agent.source === "flowpilot" && !agent.provider;
                    const cardProvBadge = isProviderAgnostic ? null : (isClaudeSource ? "CLAUDE" : "CODEX");
                    const cardProvClass = isClaudeSource ? "claude" : "codex";

                    return (
                      <div
                        key={agent.name}
                        className={`opt ${cardRoleClass} ${isAgentSelected ? "sel" : ""}`}
                        onClick={() => setDialog((current) => ({ ...(current ?? { prompt: "", wait: false, dependsOn: [] }), agentName: agent.name }))}
                      >
                        <span className="bar" />
                        <div style={{ flex: 1 }}>
                          <div className="nm">
                            {agent.name}{" "}
                            {isProviderAgnostic ? (
                              <>
                                <span className="pill-prov prov-codex">CODEX</span>{" "}
                                <span className="pill-prov prov-claude">CLAUDE</span>
                              </>
                            ) : (
                              <span className={`pill-prov prov-${cardProvClass}`}>{cardProvBadge}</span>
                            )}
                            {agent.path && <span className="src">{agent.path.split("/").pop()}</span>}
                          </div>
                          <div className="ds">{agent.description || agent.role}</div>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>

              <div className="fld">
                <label>Provider override <span className="src-hint">(optional — defaults to the agent's preference)</span></label>
                <div className="chips">
                  <span className={`chip ${!dialog?.providerOverride ? "sel" : ""}`} onClick={() => setDialog(c => c ? { ...c, providerOverride: undefined } : null)}>inherit</span>
                  <span className={`chip ${dialog?.providerOverride === "claude" ? "sel" : ""}`} onClick={() => setDialog(c => c ? { ...c, providerOverride: "claude" } : null)}>✳ Claude</span>
                  <span className={`chip ${dialog?.providerOverride === "codex" ? "sel" : ""}`} onClick={() => setDialog(c => c ? { ...c, providerOverride: "codex" } : null)}>◎ Codex</span>
                  <span className={`chip ${dialog?.providerOverride === "gemini" ? "sel" : ""}`} onClick={() => setDialog(c => c ? { ...c, providerOverride: "gemini" } : null)}>◆ Gemini</span>
                </div>
              </div>

              <div className="fld">
                <label>Task prompt</label>
                <textarea
                  className="promptbox"
                  value={dialog?.prompt ?? ""}
                  onChange={(e) => setDialog((current) => ({ ...(current ?? { agentName: "coder", wait: false, dependsOn: [] }), prompt: e.target.value }))}
                  placeholder="What should this child agent work on?"
                  style={{ width: "100%", outline: "none", resize: "vertical" }}
                />
              </div>

              <div className="fld">
                <label>Dependencies <span className="src-hint">(this agent waits until they finish)</span></label>
                <div className="chips">
                  <span className={`chip ${!dialog?.dependsOn || dialog.dependsOn.length === 0 ? "sel" : ""}`} onClick={() => setDialog(c => c ? { ...c, dependsOn: [] } : null)}>none</span>
                  {dependencyCandidates.map((dep) => {
                    const isDepSelected = dialog?.dependsOn?.includes(dep.runId);
                    return (
                      <span
                        key={dep.runId}
                        className={`chip ${isDepSelected ? "sel" : ""}`}
                        onClick={() => setDialog(c => {
                          if (!c) return null;
                          const currentDeps = c.dependsOn ?? [];
                          const nextDeps = isDepSelected ? currentDeps.filter(d => d !== dep.runId) : [...currentDeps, dep.runId];
                          return { ...c, dependsOn: nextDeps };
                        })}
                      >
                        ⟂ wait for: {dep.agentName}
                      </span>
                    );
                  })}
                </div>
              </div>

              <div className="toggle-row">
                <div
                  className={`sw-toggle ${dialog?.wait ? "" : "off"}`}
                  onClick={() => setDialog(c => c ? { ...c, wait: !c.wait } : null)}
                >
                  <i />
                </div>
                <div>
                  <b>Wait for result</b>
                  <div className="sub">
                    Off → run in the background; the agent streams in the AGENTS panel and the main turn continues.
                    On → block this turn until the agent finishes and return its final message inline.
                  </div>
                </div>
              </div>
            </div>
            <div className="dlg-f">
              <span className="hintleft">Spawns a child run under <b>main</b> · streams over its own SSE</span>
              <button type="button" className="bc" onClick={() => { setDialog(null); setOpen(false); }}>Cancel</button>
              <button
                type="button"
                className="bc primary"
                onClick={() => void spawn()}
                disabled={spawning || !dialog?.agentName || dialog.prompt.trim().length === 0}
              >
                Spawn ▸
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
