import { useEffect, useMemo, useState } from "react";
import { useStore } from "@/state/store";
import type { AgentDefinition } from "@/types/contract";

interface SpawnDialogState {
  agentName: string;
  prompt: string;
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

  const [open, setOpen] = useState(false);
  const [agents, setAgents] = useState<AgentDefinition[]>([]);
  const [loadingAgents, setLoadingAgents] = useState(false);
  const [dialog, setDialog] = useState<SpawnDialogState | null>(null);
  const [spawning, setSpawning] = useState(false);

  useEffect(() => {
    if (!agentSpawnGuideOpen) return;
    setOpen(true);
    setDialog({ agentName: agentSpawnGuideAgentName ?? "architect", prompt: "" });
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
        setDialog((current) => current ?? (result[0] ? { agentName: result[0].name, prompt: "" } : null));
      })
      .finally(() => setLoadingAgents(false));
  }, [cwd, listAgents, open]);

  useEffect(() => {
    void refreshAgentRuns();
  }, [refreshAgentRuns, mainRunId]);

  const spawn = async (): Promise<void> => {
    if (!dialog || !mainRunId || !client.spawnAgent) return;
    setSpawning(true);
    try {
      await client.spawnAgent({
        parentRunId: mainRunId,
        agent: dialog.agentName,
        prompt: dialog.prompt,
      });
      await refreshAgentRuns();
      setDialog(null);
      setOpen(false);
    } finally {
      setSpawning(false);
    }
  };

  const mainCardActive = !activeAgentRunId || activeAgentRunId === mainRunId;
  const mainRunStatus = activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId ? mainSnapshotStatus ?? activeStatus : activeStatus;
  const mainCardBusy = mainRunStatus === "running" || mainRunStatus === "waiting_approval" || mainRunStatus === "waiting_question";

  return (
    <section className="workflow-rail agent-rail">
      <div className="project-rail-head">
        <div>
          <label>Agents</label>
          <p>Spawn and focus child runs for the current main session.</p>
        </div>
      </div>

      <div className="agent-rail-actions">
        <button type="button" className="secondary-btn agent-spawn-btn" onClick={() => setOpen(true)} disabled={!mainRunId}>
          + Spawn agent
        </button>
        {activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId && (
          <button type="button" className="ghost-btn agent-back-btn" onClick={backToMainRun}>
            Back to main
          </button>
        )}
      </div>

      <div className="agent-run-list">
        <button
          type="button"
          className={`agent-run-card agent-main-card ${mainCardActive ? "active" : ""}`}
          onClick={() => mainRunId && void backToMainRun()}
          disabled={!mainRunId}
        >
          <div className="agent-run-head">
            <strong>Main run</strong>
            <span className={`agent-run-status status-${mainCardBusy ? mainRunStatus : "completed"}`}>
              {mainRunStatus ?? "idle"}
            </span>
          </div>
          <div className="agent-run-meta">
            <span>{mainRunId ?? "no active run"}</span>
            {agentRuns.length > 0 && <span>{agentRuns.filter((run) => run.status === "running" || run.status === "waiting_approval" || run.status === "waiting_question").length} active</span>}
          </div>
        </button>
        {agentRuns.length === 0 ? (
          <div className="agent-empty">{mainRunId ? "No child runs yet." : "Start a run to enable agents."}</div>
        ) : (
          agentRuns.map((run) => {
            const colorClass =
              run.role.toLowerCase().includes("arch") ? "role-arch" : run.role.toLowerCase().includes("review") ? "role-review" : "role-build";
            const isActive = run.runId === activeAgentRunId;
            return (
              <button
                key={run.runId}
                type="button"
                className={`agent-run-card ${colorClass} ${isActive ? "active" : ""}`}
                onClick={() => void focusAgentRun(run.runId)}
              >
                <div className="agent-run-head">
                  <strong>{run.agentName}</strong>
                  <span className={`agent-run-status status-${run.status}`}>{run.status}</span>
                </div>
                <div className="agent-run-meta">
                  <span>{run.role}</span>
                  <span>{new Date(run.createdAt).toLocaleString()}</span>
                </div>
                {run.dependsOn && run.dependsOn.length > 0 && <div className="agent-run-depends">depends on {run.dependsOn.join(", ")}</div>}
                {run.agentStatus && <div className="agent-run-status-line">{run.agentStatus}</div>}
              </button>
            );
          })
        )}
      </div>

      {open && (
        <div className="modal-backdrop">
          <div className="modal agent-modal">
            <div className="modal-head">
              <div>
                <div className="modal-title">Spawn agent</div>
                <p className="project-muted-copy">Spawn a child run under the main session.</p>
              </div>
              <button type="button" className="secondary-btn modal-close" onClick={() => { setDialog(null); setOpen(false); }}>
                Close
              </button>
            </div>
            <div className="settings-grid">
              <label className="settings-field">
                <span>Agent</span>
                <select value={dialog?.agentName ?? ""} disabled={loadingAgents} onChange={(e) => setDialog((current) => ({ ...(current ?? { prompt: "" }), agentName: e.target.value }))}>
                  {(agents.length > 0 ? agents : [{ name: "architect", description: "", role: "", source: "flowpilot" }]).map((agent) => (
                    <option key={agent.name} value={agent.name}>
                      {agent.name}
                    </option>
                  ))}
                </select>
              </label>
              <label className="settings-field">
                <span>Prompt</span>
                <textarea
                  value={dialog?.prompt ?? ""}
                  onChange={(e) => setDialog((current) => ({ ...(current ?? { agentName: agents[0]?.name ?? "architect" }), prompt: e.target.value }))}
                  placeholder="What should this child agent work on?"
                />
              </label>
            </div>
            <div className="modal-actions">
              <button type="button" className="primary-btn" onClick={() => void spawn()} disabled={spawning || !dialog?.agentName || dialog.prompt.trim().length === 0}>
                Spawn
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
