import { useEffect, useMemo, useState } from "react";
import { useStore } from "@/state/store";
import type { AgentDefinition, AgentRunSummary } from "@/types/contract";
import { formatDependencyLabels } from "@/components/agentDependencies";
import { BoardIcon, BotIcon, CaretIcon, CloseIcon, PlusIcon } from "@/components/icons";

interface SpawnDialogState {
  agentName: string;
  prompt: string;
  providerOverride?: string;
  dependsOn?: string[];
  wait?: boolean;
}

interface MainAgentDisplayInput {
  resolvedProvider: string;
  resolvedModel: string;
  runtimeMetaProvider: string | undefined;
  runtimeMetaModel: string | undefined;
  selectedProvider: string | undefined;
  selectedModel: string | undefined;
}

/**
 * BUG-227: once a run has started, workflowStepRuntimeMeta (runtimeMeta*) carries
 * the run's actual resolved posture (Step > Flow > Project, BUG-165) and must win
 * over resolvedProvider/resolvedModel, which are only a pre-run PREVIEW derived
 * from the picked workflow/step's catalog row. Exported so the priority order has
 * a direct regression test independent of the component's render output.
 */
export function resolveMainAgentDisplay(input: MainAgentDisplayInput): { mainProvider: string; mainModel: string } {
  const mainProvider = input.runtimeMetaProvider || input.resolvedProvider || input.selectedProvider || "codex";
  const mainModel = input.runtimeMetaModel || input.resolvedModel || input.selectedModel || "";
  return { mainProvider, mainModel };
}

export function agentRunDisplayName(run: Pick<AgentRunSummary, "agentName" | "label">): string {
  return run.label || run.agentName;
}

export function AgentsPanel(): React.ReactElement | null {
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

  const chatMode = useStore((s) => s.chatMode);
  const launchMode = useStore((s) => s.launchMode);
  const workflows = useStore((s) => s.workflows);
  const steps = useStore((s) => s.steps);
  const selectedWorkflowId = useStore((s) => s.selectedWorkflowId);
  const selectedStepId = useStore((s) => s.selectedStepId);

  const isFlowMode = chatMode === "workflow_step_auto";
  const isWorkflowSelected = launchMode === "workflow" && selectedWorkflowId;
  const isStepSelected = launchMode === "step" && selectedStepId;

  const project = useMemo(() => projects.find((p) => p.id === selectedProjectId), [projects, selectedProjectId]);
  const selectedWorkflow = useMemo(() => workflows.find((w) => w.id === selectedWorkflowId), [workflows, selectedWorkflowId]);
  const selectedStep = useMemo(() => steps.find((step) => step.id === selectedStepId), [steps, selectedStepId]);

  const resolvedModel = useMemo(() => {
    if (isFlowMode) {
      if (isWorkflowSelected) {
        return selectedWorkflow?.model || project?.model || "";
      } else if (isStepSelected) {
        return selectedStep?.model || project?.model || "";
      }
    }
    return "";
  }, [isFlowMode, isWorkflowSelected, isStepSelected, selectedWorkflow, selectedStep, project]);

  const resolvedProvider = useMemo(() => {
    if (!resolvedModel) return "";
    const m = resolvedModel.toLowerCase().trim();
    if (m.startsWith("gpt-")) return "codex";
    if (m.startsWith("gemini-") || m.startsWith("auto-gemini-")) return "gemini";
    if (m.startsWith("claude-")) return "claude";
    return "";
  }, [resolvedModel]);

  const runtimeMetaProvider = useStore((s) => s.workflowStepRuntimeMeta.provider);
  const runtimeMetaModel = useStore((s) => s.workflowStepRuntimeMeta.model);
  const selectedProvider = useStore((s) => s.selectedProvider);
  const selectedModel = useStore((s) => s.selectedModel);
  const { mainProvider, mainModel } = resolveMainAgentDisplay({
    resolvedProvider,
    resolvedModel,
    runtimeMetaProvider,
    runtimeMetaModel,
    selectedProvider,
    selectedModel,
  });
  const hideUntilFlowTargetSelected = isFlowMode && !isWorkflowSelected && !isStepSelected;

  const [open, setOpen] = useState(false);
  const [agents, setAgents] = useState<AgentDefinition[]>([]);
  const [loadingAgents, setLoadingAgents] = useState(false);
  const [dialog, setDialog] = useState<SpawnDialogState | null>(null);
  const [spawning, setSpawning] = useState(false);
  const [showAllActive, setShowAllActive] = useState(false);
  const [showAllClosed, setShowAllClosed] = useState(false);

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

  // Dedup by runId and sort newest-first.
  // (declared before the blocking check below; hasBlockingChild reads orderedRuns) The list is fed by both the orchestration
  // SSE stream and a fire-and-forget HTTP refresh, which can race and momentarily
  // double-count or reorder agents; deduping by id keeps the count stable and sorting
  // by createdAt keeps the latest agent on top (BUG-130).
  const orderedRuns = useMemo(() => {
    const byId = new Map<string, (typeof agentRuns)[number]>();
    for (const run of agentRuns) byId.set(run.runId, run);
    return [...byId.values()].sort((a, b) => (b.createdAt ?? "").localeCompare(a.createdAt ?? ""));
  }, [agentRuns]);

  const runningAgentCount = orderedRuns.filter(
    (run) => run.status === "running" || run.status === "waiting_approval" || run.status === "waiting_question",
  ).length;

  const activeRuns = orderedRuns.filter((r) => r.status !== "completed" && r.status !== "failed" && r.status !== "cancelled");
  const closedRuns = orderedRuns.filter((r) => r.status === "completed" || r.status === "failed" || r.status === "cancelled");

  // Collapse long lists to the latest few; the rest expand on demand (BUG-130).
  const AGENT_COLLAPSE_LIMIT = 5;
  const visibleActiveRuns = showAllActive ? activeRuns : activeRuns.slice(0, AGENT_COLLAPSE_LIMIT);
  const visibleClosedRuns = showAllClosed ? closedRuns : closedRuns.slice(0, AGENT_COLLAPSE_LIMIT);

  // A running child spawned with wait=true blocks the main run even when the main has no
  // turn of its own in flight (e.g. a UI wait=true spawn). Gate spawning on it so the user
  // cannot fan out concurrent agents while a blocking child is active (BUG-133).
  const hasBlockingChild = orderedRuns.some(
    (r) => r.waitForResult && (r.status === "running" || r.status === "waiting_approval" || r.status === "waiting_question"),
  );
  const spawnBlocked = mainCardBusy || hasBlockingChild;

  const dependencyCandidates = activeRuns;
  if (hideUntilFlowTargetSelected) {
    return null;
  }

  return (
    <section className="workflow-rail workflow-rail-right agents-panel">
      <div className="project-rail-head">
        <div>
          <label>
            Agents
            {runningAgentCount > 0 && <span className="acount">{runningAgentCount} running</span>}
          </label>
          <p>
            {workspaceMainView === "board"
              ? "Board is open in the main pane. Click an agent to open its chat instead."
              : "Sub-agents in this session — click to open."}
          </p>
        </div>
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
          <BoardIcon size={12} />
          <span>Board</span>
        </button>
      </div>

      <div className="ag-sec">
        Running
      </div>

      <div className="agent-run-list">
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
            <span className={`pill-prov prov-${mainProvider}`}>{mainProvider.toUpperCase()}</span> orchestrator
            {mainModel && <span className="ac-model">{mainModel}</span>}
          </div>
        </div>

        {visibleActiveRuns.map((run) => {
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
                <span className="ac-nm">{agentRunDisplayName(run)}</span>
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
                  depends: {formatDependencyLabels(run.dependsOn, agentRuns).join(", ")}
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

        {activeRuns.length > AGENT_COLLAPSE_LIMIT && (
          <button type="button" className="ag-more" onClick={() => setShowAllActive((v) => !v)}>
            <CaretIcon open={showAllActive} /> {showAllActive ? "Show fewer" : `Show ${activeRuns.length - AGENT_COLLAPSE_LIMIT} more`}
          </button>
        )}

        <button
          type="button"
          className="spawn"
          onClick={() => setOpen(true)}
          disabled={!mainRunId || !client.spawnAgent || spawnBlocked}
          title={spawnBlocked ? "Main is busy (a turn or a wait=true agent is running) — wait for it to finish before spawning another." : undefined}
        >
          <PlusIcon size={12} /> Spawn agent
        </button>
      </div>

      {closedRuns.length > 0 && (
        <>
          <div className="ag-sec">
            Recently closed
          </div>
          <div className="agent-run-list">
            {visibleClosedRuns.map((run) => {
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
                    <span className="ac-nm">{agentRunDisplayName(run)}</span>
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
            {closedRuns.length > AGENT_COLLAPSE_LIMIT && (
              <button type="button" className="ag-more" onClick={() => setShowAllClosed((v) => !v)}>
                <CaretIcon open={showAllClosed} /> {showAllClosed ? "Show fewer" : `Show ${closedRuns.length - AGENT_COLLAPSE_LIMIT} more`}
              </button>
            )}
          </div>
        </>
      )}

      {open && (
        <div className="modal-backdrop" onClick={() => { setDialog(null); setOpen(false); }}>
          <div className="dlg" onClick={(e) => e.stopPropagation()}>
            <div className="dlg-h">
              <span className="dlg-h-title"><BotIcon size={14} /> Spawn sub-agent</span>
              <button type="button" className="x" onClick={() => { setDialog(null); setOpen(false); }} aria-label="Close spawn dialog"><CloseIcon size={12} /></button>
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
                    const isOpencodeSource = agent.source === "opencode" ||
                      (agent.source === "provider" && lowerPath.includes(".opencode"));
                    const isDevinSource = agent.source === "devin" ||
                      (agent.source === "provider" && lowerPath.includes(".devin"));
                    const isProviderAgnostic = agent.source === "flowpilot" && !agent.provider;
                    const cardProvBadge = isProviderAgnostic ? null : (isDevinSource ? "DEVIN" : isOpencodeSource ? "OPENCODE" : isClaudeSource ? "CLAUDE" : "CODEX");
                    const cardProvClass = isDevinSource ? "devin" : isOpencodeSource ? "opencode" : isClaudeSource ? "claude" : "codex";

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
                  <span className={`chip ${dialog?.providerOverride === "claude" ? "sel" : ""}`} onClick={() => setDialog(c => c ? { ...c, providerOverride: "claude" } : null)}>Claude</span>
                  <span className={`chip ${dialog?.providerOverride === "codex" ? "sel" : ""}`} onClick={() => setDialog(c => c ? { ...c, providerOverride: "codex" } : null)}>Codex</span>
                  <span className={`chip ${dialog?.providerOverride === "gemini" ? "sel" : ""}`} onClick={() => setDialog(c => c ? { ...c, providerOverride: "gemini" } : null)}>Gemini</span>
                  <span className={`chip ${dialog?.providerOverride === "grok" ? "sel" : ""}`} onClick={() => setDialog(c => c ? { ...c, providerOverride: "grok" } : null)}>Grok</span>
                  <span className={`chip ${dialog?.providerOverride === "opencode" ? "sel" : ""}`} onClick={() => setDialog(c => c ? { ...c, providerOverride: "opencode" } : null)}>OpenCode</span>
                  <span className={`chip ${dialog?.providerOverride === "devin" ? "sel" : ""}`} onClick={() => setDialog(c => c ? { ...c, providerOverride: "devin" } : null)}>Devin</span>
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
                        wait for: {dep.agentName}
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
                disabled={spawning || spawnBlocked || !dialog?.agentName || dialog.prompt.trim().length === 0}
                title={spawnBlocked ? "Main is busy (a turn or a wait=true agent is running) — wait for it to finish." : undefined}
              >
                Spawn
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
