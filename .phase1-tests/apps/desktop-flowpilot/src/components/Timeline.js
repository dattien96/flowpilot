"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.shouldShowAgentTimelineHeader = shouldShowAgentTimelineHeader;
exports.Timeline = Timeline;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const store_1 = require("@/state/store");
const TIMELINE_PAGE_SIZE = 6;
function countPrompts(timeline) {
    return timeline.filter((item) => item.kind === "prompt").length;
}
function sliceTimelineFromPrompt(timeline, visiblePromptCount) {
    const totalPrompts = countPrompts(timeline);
    if (visiblePromptCount >= totalPrompts) {
        return timeline;
    }
    const promptsToSkip = totalPrompts - visiblePromptCount;
    let promptsSeen = 0;
    for (let i = 0; i < timeline.length; i++) {
        if (timeline[i].kind === "prompt") {
            promptsSeen++;
            if (promptsSeen > promptsToSkip) {
                return timeline.slice(i);
            }
        }
    }
    return timeline;
}
const ApprovalCard_1 = require("./ApprovalCard");
const QuestionCard_1 = require("./QuestionCard");
const shortName = (path) => path.split("/").pop() ?? path;
const TOOL_ICON = { running: "⏳", success: "✓", failed: "✕", cancelled: "⊘" };
const FILE_ICON = { created: "＋", modified: "✎", deleted: "－", renamed: "→" };
function shouldShowAgentTimelineHeader(activeAgentRunId, mainRunId, agentRunCount) {
    return Boolean(activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId) || agentRunCount > 0;
}
function previewValue(value) {
    if (value === undefined || value === null)
        return undefined;
    if (typeof value === "string")
        return value;
    if (Array.isArray(value))
        return value.map((part) => (typeof part === "string" ? part : JSON.stringify(part))).join(" ");
    if (typeof value === "object") {
        const record = value;
        const command = record.command ?? record.cmd ?? record.name ?? record.path;
        if (typeof command === "string")
            return command;
        if (Array.isArray(command))
            return command.map((part) => String(part)).join(" ");
        try {
            return JSON.stringify(value);
        }
        catch {
            return String(value);
        }
    }
    return String(value);
}
function toolLabel(tool) {
    const name = typeof tool.toolName === "string" ? tool.toolName.trim() : "";
    if (name && name !== "command" && name !== "shell")
        return name;
    return previewValue(tool.input) ?? previewValue(tool.output) ?? (name || `tool ${tool.id || "call"}`);
}
function CopyBubble({ text, className, children, }) {
    const [copied, setCopied] = (0, react_1.useState)(false);
    const copy = async () => {
        try {
            await navigator.clipboard.writeText(text);
            setCopied(true);
            window.setTimeout(() => setCopied(false), 1200);
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] copy failed:", err);
        }
    };
    return ((0, jsx_runtime_1.jsxs)("div", { className: `${className} copyable-bubble`, children: [(0, jsx_runtime_1.jsx)("button", { type: "button", className: "bubble-copy", onClick: copy, "aria-label": copied ? "Copied" : "Copy message", title: copied ? "Copied" : "Copy", children: copied ? "✓" : "⧉" }), children] }));
}
function PromptSkillsSummary({ skills }) {
    const [open, setOpen] = (0, react_1.useState)(false);
    const summary = `${skills.length} skill${skills.length === 1 ? "" : "s"} selected`;
    const preview = skills.slice(0, 2).map((name) => `/${name}`).join(", ");
    const remainder = skills.length - 2;
    return ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)("button", { type: "button", className: `prompt-skills-summary ${open ? "prompt-skills-summary-open" : ""}`, "aria-expanded": open, "aria-label": summary, title: summary, onClick: () => setOpen((value) => !value), children: [(0, jsx_runtime_1.jsx)("span", { className: "prompt-skills-caret", "aria-hidden": "true", children: open ? "▾" : "▸" }), (0, jsx_runtime_1.jsx)("span", { className: "prompt-skills-title", children: summary }), (0, jsx_runtime_1.jsxs)("span", { className: "prompt-skills-preview", children: [preview, remainder > 0 ? ` +${remainder}` : ""] })] }), open && ((0, jsx_runtime_1.jsx)("div", { className: "prompt-skills-body", children: skills.map((name) => ((0, jsx_runtime_1.jsx)("div", { className: "prompt-skill-chip", children: (0, jsx_runtime_1.jsxs)("span", { className: "prompt-skill-name", children: ["/", name] }) }, name))) }))] }));
}
function ToolRow({ it }) {
    const label = toolLabel(it);
    const output = previewValue(it.output);
    const status = it.status || "success";
    return ((0, jsx_runtime_1.jsxs)("div", { className: `row tool-row tool-${status}`, children: [(0, jsx_runtime_1.jsx)("span", { className: "row-icon", children: TOOL_ICON[status] ?? "•" }), (0, jsx_runtime_1.jsxs)("span", { className: "row-main", children: [(0, jsx_runtime_1.jsx)("code", { className: "tool-label", children: label }), status !== "running" && (0, jsx_runtime_1.jsx)("span", { className: "row-status", children: status }), output && output !== label && (0, jsx_runtime_1.jsx)("span", { className: "row-output", children: output })] })] }));
}
function toolGroupLabel(tools) {
    if (tools.some((tool) => tool.status === "running"))
        return `${tools.length} tool call${tools.length === 1 ? "" : "s"} running`;
    if (tools.some((tool) => tool.status === "failed"))
        return `${tools.length} tool call${tools.length === 1 ? "" : "s"} · failed`;
    if (tools.some((tool) => tool.status === "cancelled"))
        return `${tools.length} tool call${tools.length === 1 ? "" : "s"} · cancelled`;
    return `${tools.length} tool call${tools.length === 1 ? "" : "s"} completed`;
}
function ToolGroup({ tools }) {
    const [open, setOpen] = (0, react_1.useState)(false);
    const label = toolGroupLabel(tools);
    return ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)("button", { type: "button", className: `tool-group-summary ${open ? "tool-group-summary-open" : ""}`, "aria-expanded": open, "aria-label": label, title: label, onClick: () => setOpen((value) => !value), children: [(0, jsx_runtime_1.jsx)("span", { className: "tool-group-caret", children: open ? "▾" : "▸" }), (0, jsx_runtime_1.jsx)("span", { className: "tool-group-title", children: label })] }), open && ((0, jsx_runtime_1.jsx)("div", { className: "tool-group-body", children: tools.length === 0 ? ((0, jsx_runtime_1.jsx)("div", { className: "tool-empty", children: "No tool calls captured." })) : (tools.map((tool, index) => (0, jsx_runtime_1.jsx)(ToolRow, { it: tool }, tool.id || `tool-${index}`))) }))] }));
}
function FileRow({ it }) {
    const openInIde = (0, store_1.useStore)((s) => s.openInIde);
    return ((0, jsx_runtime_1.jsxs)("button", { className: "row file-row", title: it.path, onClick: () => openInIde(it.path), children: [(0, jsx_runtime_1.jsx)("span", { className: "row-icon", children: FILE_ICON[it.changeType ?? "modified"] ?? "✎" }), (0, jsx_runtime_1.jsxs)("span", { className: "row-main", children: [(0, jsx_runtime_1.jsx)("span", { className: "file-name", children: shortName(it.path) }), (0, jsx_runtime_1.jsx)("span", { className: "file-change", children: it.changeType ?? "modified" })] }), (0, jsx_runtime_1.jsx)("span", { className: "row-hint", children: "open in IDE \u2197" })] }));
}
function buildTimelineGroups(timeline) {
    const groups = [];
    let pendingTools = [];
    const flushTools = () => {
        if (pendingTools.length === 0)
            return;
        const firstToolId = pendingTools[0].id || `${pendingTools[0].toolName}-${groups.length}`;
        groups.push({ kind: "tool-group", id: `tool-group-${firstToolId}`, tools: pendingTools });
        pendingTools = [];
    };
    for (const item of timeline) {
        if (item.kind === "tool") {
            pendingTools.push(item);
        }
        else {
            flushTools();
            groups.push(item);
        }
    }
    flushTools();
    return groups;
}
function Item({ it }) {
    switch (it.kind) {
        case "assistant":
            return ((0, jsx_runtime_1.jsxs)(CopyBubble, { text: it.text, className: `bubble assistant ${it.finalized ? "final" : "streaming"}`, children: [it.text, !it.finalized && (0, jsx_runtime_1.jsx)("span", { className: "caret", children: "\u258C" })] }));
        case "prompt":
            return ((0, jsx_runtime_1.jsxs)("div", { className: "prompt-stack", children: [it.attachments && it.attachments.length > 0 && ((0, jsx_runtime_1.jsx)("div", { className: "prompt-attachments", "aria-label": `${it.attachments.length} image attachment(s)`, children: it.attachments.map((att) => att.previewUrl ? ((0, jsx_runtime_1.jsx)("img", { className: "prompt-attachment-thumb", src: att.previewUrl, alt: att.originalName, title: att.originalName }, att.id)) : ((0, jsx_runtime_1.jsxs)("span", { className: "prompt-attachment-chip", title: att.originalName, children: ["\uD83D\uDDBC ", att.originalName] }, att.id))) })), (0, jsx_runtime_1.jsx)(CopyBubble, { text: it.text, className: "bubble prompt", children: it.text }), it.selectedSkills && it.selectedSkills.length > 0 && ((0, jsx_runtime_1.jsx)(PromptSkillsSummary, { skills: it.selectedSkills }))] }));
        case "thinking":
            return (0, jsx_runtime_1.jsx)("div", { className: "system-line thinking", children: it.text });
        case "tool":
            return (0, jsx_runtime_1.jsx)(ToolRow, { it: it });
        case "tool-group":
            return (0, jsx_runtime_1.jsx)(ToolGroup, { tools: it.tools });
        case "file":
            return (0, jsx_runtime_1.jsx)(FileRow, { it: it });
        case "approval":
            return (0, jsx_runtime_1.jsx)(ApprovalCard_1.ApprovalCard, { approvalId: it.approvalId, details: it.details, decision: it.decision });
        case "question":
            return ((0, jsx_runtime_1.jsx)(QuestionCard_1.QuestionCard, { questionId: it.questionId, prompt: it.prompt, options: it.options, multiSelect: it.multiSelect, answer: it.answer }));
        case "system":
            return (0, jsx_runtime_1.jsx)("div", { className: `system-line ${it.tone}`, children: it.text });
        default:
            return null;
    }
}
function Timeline() {
    const timeline = (0, store_1.useStore)((s) => s.timeline);
    const mainRunId = (0, store_1.useStore)((s) => s.mainRunId ?? s.runId);
    const activeAgentRunId = (0, store_1.useStore)((s) => s.activeAgentRunId);
    const agentRuns = (0, store_1.useStore)((s) => s.agentRuns);
    const backToMainRun = (0, store_1.useStore)((s) => s.backToMainRun);
    const endRef = (0, react_1.useRef)(null);
    const totalPromptCount = countPrompts(timeline);
    const [visiblePromptCount, setVisiblePromptCount] = (0, react_1.useState)(TIMELINE_PAGE_SIZE);
    (0, react_1.useEffect)(() => {
        setVisiblePromptCount((current) => {
            if (totalPromptCount <= TIMELINE_PAGE_SIZE) {
                return totalPromptCount;
            }
            return Math.min(Math.max(current, TIMELINE_PAGE_SIZE), totalPromptCount);
        });
    }, [totalPromptCount]);
    const hiddenPromptCount = Math.max(totalPromptCount - visiblePromptCount, 0);
    const visibleTimeline = sliceTimelineFromPrompt(timeline, visiblePromptCount);
    const timelineGroups = buildTimelineGroups(visibleTimeline);
    const runningAgentCount = agentRuns.filter((run) => run.status === "running" || run.status === "waiting_approval" || run.status === "waiting_question").length;
    const showAgentHeader = shouldShowAgentTimelineHeader(activeAgentRunId, mainRunId, agentRuns.length);
    (0, react_1.useEffect)(() => {
        endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
    }, [timeline]);
    return ((0, jsx_runtime_1.jsxs)("div", { className: `timeline ${activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId ? "timeline-agent-focused" : ""}`, children: [showAgentHeader && ((0, jsx_runtime_1.jsxs)("div", { className: "timeline-head", children: [activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId ? ((0, jsx_runtime_1.jsxs)("div", { className: "timeline-breadcrumb", children: [(0, jsx_runtime_1.jsx)("button", { type: "button", className: "timeline-back-btn", onClick: backToMainRun, children: "\u2190 Back to main" }), (0, jsx_runtime_1.jsx)("span", { className: "timeline-breadcrumb-sep", children: "/" }), (0, jsx_runtime_1.jsx)("span", { className: "timeline-breadcrumb-current", children: activeAgentRunId })] })) : ((0, jsx_runtime_1.jsx)("div", { className: "timeline-breadcrumb", children: (0, jsx_runtime_1.jsx)("span", { className: "timeline-breadcrumb-current", children: runningAgentCount > 0 ? `${runningAgentCount} agents running` : "Main run" }) })), runningAgentCount > 0 && ((0, jsx_runtime_1.jsx)("div", { className: "timeline-agent-summary", children: (0, jsx_runtime_1.jsxs)("span", { children: [runningAgentCount, " agents running"] }) }))] })), timeline.length === 0 && (0, jsx_runtime_1.jsx)("div", { className: "empty", children: "Select a project, choose your chat controls, and send a prompt to begin." }), hiddenPromptCount > 0 && ((0, jsx_runtime_1.jsxs)("button", { type: "button", className: "load-earlier-btn", onClick: () => setVisiblePromptCount((current) => Math.min(totalPromptCount, current + TIMELINE_PAGE_SIZE)), children: ["\u2191 Load earlier prompts (", hiddenPromptCount, ")"] })), timelineGroups.map((it) => ((0, jsx_runtime_1.jsx)(Item, { it: it }, it.id))), (0, jsx_runtime_1.jsx)("div", { ref: endRef })] }));
}
