"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.formatDependencyLabels = formatDependencyLabels;
exports.AgentsPanel = AgentsPanel;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const store_1 = require("@/state/store");
function formatDependencyLabels(dependsOn, agentRuns) {
    if (!dependsOn || dependsOn.length === 0)
        return [];
    const namesByRunId = new Map(agentRuns.map((run) => [run.runId, run.agentName]));
    return dependsOn.map((runId) => namesByRunId.get(runId) ?? runId);
}
function AgentsPanel() {
    const client = (0, store_1.useStore)((s) => s.client);
    const projects = (0, store_1.useStore)((s) => s.projects);
    const selectedProjectId = (0, store_1.useStore)((s) => s.selectedProjectId);
    const mainRunId = (0, store_1.useStore)((s) => s.mainRunId ?? s.runId);
    const activeStatus = (0, store_1.useStore)((s) => s.status);
    const mainSnapshotStatus = (0, store_1.useStore)((s) => (mainRunId ? s._runSnapshots[mainRunId]?.status : undefined));
    const activeAgentRunId = (0, store_1.useStore)((s) => s.activeAgentRunId);
    const agentRuns = (0, store_1.useStore)((s) => s.agentRuns);
    const agentSpawnGuideOpen = (0, store_1.useStore)((s) => s.agentSpawnGuideOpen);
    const agentSpawnGuideAgentName = (0, store_1.useStore)((s) => s.agentSpawnGuideAgentName);
    const refreshAgentRuns = (0, store_1.useStore)((s) => s.refreshAgentRuns);
    const focusAgentRun = (0, store_1.useStore)((s) => s.focusAgentRun);
    const backToMainRun = (0, store_1.useStore)((s) => s.backToMainRun);
    const listAgents = (0, store_1.useStore)((s) => s.listAgents);
    const clearAgentSpawnGuide = (0, store_1.useStore)((s) => s.clearAgentSpawnGuide);
    const workspaceMainView = (0, store_1.useStore)((s) => s.workspaceMainView);
    const openOrchestrationBoard = (0, store_1.useStore)((s) => s.openOrchestrationBoard);
    const closeOrchestrationBoard = (0, store_1.useStore)((s) => s.closeOrchestrationBoard);
    const [open, setOpen] = (0, react_1.useState)(false);
    const [agents, setAgents] = (0, react_1.useState)([]);
    const [loadingAgents, setLoadingAgents] = (0, react_1.useState)(false);
    const [dialog, setDialog] = (0, react_1.useState)(null);
    const [spawning, setSpawning] = (0, react_1.useState)(false);
    (0, react_1.useEffect)(() => {
        if (!agentSpawnGuideOpen)
            return;
        setOpen(true);
        setDialog({ agentName: agentSpawnGuideAgentName ?? "coder", prompt: "", wait: false, dependsOn: [] });
        clearAgentSpawnGuide();
    }, [agentSpawnGuideAgentName, agentSpawnGuideOpen, clearAgentSpawnGuide]);
    const cwd = (0, react_1.useMemo)(() => projects.find((project) => project.id === selectedProjectId)?.path, [projects, selectedProjectId]);
    (0, react_1.useEffect)(() => {
        if (!open)
            return;
        setLoadingAgents(true);
        void listAgents(cwd)
            .then((result) => {
            setAgents(result);
            setDialog((current) => current ?? (result[0] ? { agentName: result[0].name, prompt: "", wait: false, dependsOn: [] } : null));
        })
            .finally(() => setLoadingAgents(false));
    }, [cwd, listAgents, open]);
    (0, react_1.useEffect)(() => {
        void refreshAgentRuns();
    }, [refreshAgentRuns, mainRunId]);
    const spawn = async () => {
        if (!dialog || !mainRunId || !client.spawnAgent)
            return;
        setSpawning(true);
        try {
            await client.spawnAgent({
                parentRunId: mainRunId,
                agent: dialog.agentName,
                prompt: dialog.prompt,
                provider: dialog.providerOverride,
                dependsOn: dialog.dependsOn,
                wait: dialog.wait,
            });
            await refreshAgentRuns();
            setDialog(null);
            setOpen(false);
        }
        finally {
            setSpawning(false);
        }
    };
    const mainCardActive = !activeAgentRunId || activeAgentRunId === mainRunId;
    const mainRunStatus = activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId ? mainSnapshotStatus ?? activeStatus : activeStatus;
    const mainCardBusy = mainRunStatus === "running" || mainRunStatus === "waiting_approval" || mainRunStatus === "waiting_question";
    const runningAgentCount = agentRuns.filter((run) => run.status === "running" || run.status === "waiting_approval" || run.status === "waiting_question").length;
    const activeRuns = agentRuns.filter((r) => r.status !== "completed" && r.status !== "failed" && r.status !== "cancelled");
    const closedRuns = agentRuns.filter((r) => r.status === "completed" || r.status === "failed" || r.status === "cancelled");
    const dependencyCandidates = activeRuns;
    return ((0, jsx_runtime_1.jsxs)("section", { className: "panel agents", children: [(0, jsx_runtime_1.jsxs)("div", { className: "panel-h", children: [(0, jsx_runtime_1.jsx)("span", { children: "Agents" }), runningAgentCount > 0 && (0, jsx_runtime_1.jsxs)("span", { className: "acount", children: [runningAgentCount, " running"] }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: `board-link-btn ${workspaceMainView === "board" ? "active" : ""}`, onClick: () => {
                            if (workspaceMainView === "board") {
                                closeOrchestrationBoard();
                            }
                            else {
                                openOrchestrationBoard();
                            }
                        }, children: "\u25A6 Board" })] }), (0, jsx_runtime_1.jsx)("p", { className: "panel-sub", children: workspaceMainView === "board"
                    ? "Board is open in the main pane. Click an agent to open its chat instead."
                    : "Sub-agents in this session — click to open · ＋ to spawn." }), (0, jsx_runtime_1.jsx)("div", { className: "ag-sec", children: "\u25CF Running" }), (0, jsx_runtime_1.jsxs)("div", { className: "agent-run-list", style: { display: "flex", flexDirection: "column", gap: "6px", padding: 0 }, children: [(0, jsx_runtime_1.jsxs)("div", { className: `acard main ${mainCardActive && workspaceMainView !== "board" ? "sel" : ""}`, onClick: () => mainRunId && void backToMainRun(), children: [(0, jsx_runtime_1.jsxs)("div", { className: "ac-top", children: [(0, jsx_runtime_1.jsx)("span", { className: "ac-nm", children: "main" }), (0, jsx_runtime_1.jsxs)("span", { className: "ac-st", children: [(0, jsx_runtime_1.jsx)("span", { className: `sd ${mainCardBusy ? "run" : "done"}` }), mainCardBusy ? "active" : "idle"] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "ac-meta", children: [(0, jsx_runtime_1.jsx)("span", { className: "pill-prov prov-codex", children: "CODEX" }), " orchestrator"] })] }), activeRuns.map((run) => {
                        const isSelected = run.runId === activeAgentRunId && workspaceMainView !== "board";
                        const lowerName = run.agentName.toLowerCase();
                        const roleClass = lowerName.includes("coder") ? "coder" : lowerName.includes("review") ? "reviewer" : lowerName.includes("test") ? "tester" : "";
                        const statusClass = run.status === "running" ? "run" : (run.status === "waiting_approval" || run.status === "waiting_question") ? "wait" : "done";
                        const providerBadge = "CODEX";
                        const provClass = "codex";
                        return ((0, jsx_runtime_1.jsxs)("div", { className: `acard ${roleClass} ${isSelected ? "sel" : ""}`, onClick: () => void focusAgentRun(run.runId), children: [(0, jsx_runtime_1.jsxs)("div", { className: "ac-top", children: [(0, jsx_runtime_1.jsx)("span", { className: "ac-nm", children: run.agentName }), (0, jsx_runtime_1.jsxs)("span", { className: "ac-st", children: [(0, jsx_runtime_1.jsx)("span", { className: `sd ${statusClass}` }), run.status] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "ac-meta", children: [(0, jsx_runtime_1.jsx)("span", { className: `pill-prov prov-${provClass}`, children: providerBadge }), (0, jsx_runtime_1.jsx)("span", { children: run.role })] }), run.dependsOn && run.dependsOn.length > 0 && ((0, jsx_runtime_1.jsxs)("div", { className: "ac-meta", style: { marginTop: "3px" }, children: ["\u27C2 depends: ", formatDependencyLabels(run.dependsOn, agentRuns).join(", ")] })), run.agentStatus && ((0, jsx_runtime_1.jsx)("div", { className: "ac-last", children: run.agentStatus }))] }, run.runId));
                    }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "spawn", onClick: () => setOpen(true), disabled: !mainRunId || !client.spawnAgent, children: "\uFF0B Spawn agent" })] }), closedRuns.length > 0 && ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)("div", { className: "ag-sec", children: "\u25CC Recently closed" }), (0, jsx_runtime_1.jsx)("div", { className: "agent-run-list", style: { display: "flex", flexDirection: "column", gap: "6px", padding: 0 }, children: closedRuns.map((run) => {
                            const isSelected = run.runId === activeAgentRunId && workspaceMainView !== "board";
                            const lowerName = run.agentName.toLowerCase();
                            const providerBadge = "CODEX";
                            const provClass = "codex";
                            return ((0, jsx_runtime_1.jsxs)("div", { className: `acard closed ${isSelected ? "sel" : ""}`, onClick: () => void focusAgentRun(run.runId), children: [(0, jsx_runtime_1.jsxs)("div", { className: "ac-top", children: [(0, jsx_runtime_1.jsx)("span", { className: "ac-nm", children: run.agentName }), (0, jsx_runtime_1.jsxs)("span", { className: "ac-st", children: [(0, jsx_runtime_1.jsx)("span", { className: "sd closed" }), run.status] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "ac-meta", children: [(0, jsx_runtime_1.jsx)("span", { className: `pill-prov prov-${provClass}`, children: providerBadge }), (0, jsx_runtime_1.jsx)("span", { children: run.role })] })] }, run.runId));
                        }) })] })), open && ((0, jsx_runtime_1.jsx)("div", { className: "modal-backdrop", onClick: () => { setDialog(null); setOpen(false); }, children: (0, jsx_runtime_1.jsxs)("div", { className: "dlg", onClick: (e) => e.stopPropagation(), children: [(0, jsx_runtime_1.jsxs)("div", { className: "dlg-h", children: [(0, jsx_runtime_1.jsx)("span", { children: "\uD83E\uDD16 Spawn sub-agent" }), (0, jsx_runtime_1.jsx)("span", { className: "x", onClick: () => { setDialog(null); setOpen(false); }, children: "\u2715" })] }), (0, jsx_runtime_1.jsxs)("div", { className: "dlg-b", children: [(0, jsx_runtime_1.jsxs)("div", { className: "fld", children: [(0, jsx_runtime_1.jsxs)("label", { children: ["Agent ", (0, jsx_runtime_1.jsx)("span", { className: "src-hint", children: "\u2014 from .claude/agents \u00B7 .codex/agents \u00B7 built-ins" })] }), (0, jsx_runtime_1.jsx)("div", { style: { display: "flex", flexDirection: "column", gap: "8px" }, children: (agents.length > 0 ? agents : [
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
                                                const cardProvBadge = isClaudeSource ? "CLAUDE" : "CODEX";
                                                const cardProvClass = isClaudeSource ? "claude" : "codex";
                                                return ((0, jsx_runtime_1.jsxs)("div", { className: `opt ${cardRoleClass} ${isAgentSelected ? "sel" : ""}`, onClick: () => setDialog((current) => ({ ...(current ?? { prompt: "", wait: false, dependsOn: [] }), agentName: agent.name })), children: [(0, jsx_runtime_1.jsx)("span", { className: "bar" }), (0, jsx_runtime_1.jsxs)("div", { style: { flex: 1 }, children: [(0, jsx_runtime_1.jsxs)("div", { className: "nm", children: [agent.name, " ", (0, jsx_runtime_1.jsx)("span", { className: `pill-prov prov-${cardProvClass}`, children: cardProvBadge }), agent.path && (0, jsx_runtime_1.jsx)("span", { className: "src", children: agent.path.split("/").pop() })] }), (0, jsx_runtime_1.jsx)("div", { className: "ds", children: agent.description || agent.role })] })] }, agent.name));
                                            }) })] }), (0, jsx_runtime_1.jsxs)("div", { className: "fld", children: [(0, jsx_runtime_1.jsxs)("label", { children: ["Provider override ", (0, jsx_runtime_1.jsx)("span", { className: "src-hint", children: "(optional \u2014 defaults to the agent's preference)" })] }), (0, jsx_runtime_1.jsxs)("div", { className: "chips", children: [(0, jsx_runtime_1.jsx)("span", { className: `chip ${!dialog?.providerOverride ? "sel" : ""}`, onClick: () => setDialog(c => c ? { ...c, providerOverride: undefined } : null), children: "inherit" }), (0, jsx_runtime_1.jsx)("span", { className: `chip ${dialog?.providerOverride === "claude" ? "sel" : ""}`, onClick: () => setDialog(c => c ? { ...c, providerOverride: "claude" } : null), children: "\u2733 Claude" }), (0, jsx_runtime_1.jsx)("span", { className: `chip ${dialog?.providerOverride === "codex" ? "sel" : ""}`, onClick: () => setDialog(c => c ? { ...c, providerOverride: "codex" } : null), children: "\u25CE Codex" }), (0, jsx_runtime_1.jsx)("span", { className: `chip ${dialog?.providerOverride === "gemini" ? "sel" : ""}`, onClick: () => setDialog(c => c ? { ...c, providerOverride: "gemini" } : null), children: "\u25C6 Gemini" })] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "fld", children: [(0, jsx_runtime_1.jsx)("label", { children: "Task prompt" }), (0, jsx_runtime_1.jsx)("textarea", { className: "promptbox", value: dialog?.prompt ?? "", onChange: (e) => setDialog((current) => ({ ...(current ?? { agentName: "coder", wait: false, dependsOn: [] }), prompt: e.target.value })), placeholder: "What should this child agent work on?", style: { width: "100%", outline: "none", resize: "vertical" } })] }), (0, jsx_runtime_1.jsxs)("div", { className: "fld", children: [(0, jsx_runtime_1.jsxs)("label", { children: ["Dependencies ", (0, jsx_runtime_1.jsx)("span", { className: "src-hint", children: "(this agent waits until they finish)" })] }), (0, jsx_runtime_1.jsxs)("div", { className: "chips", children: [(0, jsx_runtime_1.jsx)("span", { className: `chip ${!dialog?.dependsOn || dialog.dependsOn.length === 0 ? "sel" : ""}`, onClick: () => setDialog(c => c ? { ...c, dependsOn: [] } : null), children: "none" }), dependencyCandidates.map((dep) => {
                                                    const isDepSelected = dialog?.dependsOn?.includes(dep.runId);
                                                    return ((0, jsx_runtime_1.jsxs)("span", { className: `chip ${isDepSelected ? "sel" : ""}`, onClick: () => setDialog(c => {
                                                            if (!c)
                                                                return null;
                                                            const currentDeps = c.dependsOn ?? [];
                                                            const nextDeps = isDepSelected ? currentDeps.filter(d => d !== dep.runId) : [...currentDeps, dep.runId];
                                                            return { ...c, dependsOn: nextDeps };
                                                        }), children: ["\u27C2 wait for: ", dep.agentName] }, dep.runId));
                                                })] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "toggle-row", children: [(0, jsx_runtime_1.jsx)("div", { className: `sw-toggle ${dialog?.wait ? "" : "off"}`, onClick: () => setDialog(c => c ? { ...c, wait: !c.wait } : null), children: (0, jsx_runtime_1.jsx)("i", {}) }), (0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("b", { children: "Wait for result" }), (0, jsx_runtime_1.jsx)("div", { className: "sub", children: "Off \u2192 run in the background; the agent streams in the AGENTS panel and the main turn continues. On \u2192 block this turn until the agent finishes and return its final message inline." })] })] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "dlg-f", children: [(0, jsx_runtime_1.jsxs)("span", { className: "hintleft", children: ["Spawns a child run under ", (0, jsx_runtime_1.jsx)("b", { children: "main" }), " \u00B7 streams over its own SSE"] }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc", onClick: () => { setDialog(null); setOpen(false); }, children: "Cancel" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "bc primary", onClick: () => void spawn(), disabled: spawning || !dialog?.agentName || dialog.prompt.trim().length === 0, children: "Spawn \u25B8" })] })] }) }))] }));
}
