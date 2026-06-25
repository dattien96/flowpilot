"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.shouldShowAgentTimelineHeader = shouldShowAgentTimelineHeader;
exports.Timeline = Timeline;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const store_1 = require("@/state/store");
const HEADING_TAGS = ["h1", "h2", "h3", "h4", "h5", "h6"];
function renderInline(text, keyPrefix) {
    const parts = [];
    const re = /(`[^`]+`|\*\*[^*\n]+\*\*|__[^_\n]+__|(?<!\*)\*(?!\*)([^*\n]+)(?<!\*)\*(?!\*)|(?<!_)_(?!_)([^_\n]+)(?<!_)_(?!_))/g;
    let last = 0;
    let m;
    let idx = 0;
    while ((m = re.exec(text)) !== null) {
        if (m.index > last)
            parts.push(text.slice(last, m.index));
        const raw = m[0];
        const key = `${keyPrefix}-${idx++}`;
        if (raw.startsWith("`")) {
            parts.push((0, jsx_runtime_1.jsx)("code", { className: "md-icode", children: raw.slice(1, -1) }, key));
        }
        else if (raw.startsWith("**") || raw.startsWith("__")) {
            parts.push((0, jsx_runtime_1.jsx)("strong", { children: raw.slice(2, -2) }, key));
        }
        else {
            parts.push((0, jsx_runtime_1.jsx)("em", { children: raw.slice(1, -1) }, key));
        }
        last = m.index + raw.length;
    }
    if (last < text.length)
        parts.push(text.slice(last));
    return parts.length === 0 ? "" : parts.length === 1 && typeof parts[0] === "string" ? parts[0] : (0, jsx_runtime_1.jsx)(jsx_runtime_1.Fragment, { children: parts });
}
function parseMdBlocks(text) {
    const blocks = [];
    const lines = text.split("\n");
    let i = 0;
    while (i < lines.length) {
        const line = lines[i];
        // Fenced code block
        const fenceMatch = /^(`{3,}|~{3,})(\S*)/.exec(line);
        if (fenceMatch) {
            const fence = fenceMatch[1];
            const lang = fenceMatch[2] ?? "";
            const codeLines = [];
            i++;
            while (i < lines.length && !lines[i].startsWith(fence)) {
                codeLines.push(lines[i]);
                i++;
            }
            blocks.push({ kind: "code", lang, content: codeLines.join("\n") });
            i++;
            continue;
        }
        // ATX heading
        const hm = /^(#{1,6})\s+(.*)$/.exec(line);
        if (hm) {
            blocks.push({ kind: "heading", level: hm[1].length, text: hm[2] });
            i++;
            continue;
        }
        // Horizontal rule (---, ***, ___)
        if (/^[-*_]{3,}\s*$/.test(line) && new Set(line.trim().split("")).size === 1) {
            blocks.push({ kind: "hr" });
            i++;
            continue;
        }
        // Table: first line starts with |
        if (line.trimStart().startsWith("|")) {
            const tableLines = [];
            while (i < lines.length && lines[i].trimStart().startsWith("|")) {
                tableLines.push(lines[i]);
                i++;
            }
            const parseRow = (r) => r.split("|").slice(1, -1).map((c) => c.trim());
            const isSeparator = (r) => /^[\s|:-]+$/.test(r);
            const hasHeader = tableLines.length >= 2 && isSeparator(tableLines[1]);
            const headers = parseRow(tableLines[0]);
            const dataRows = (hasHeader ? tableLines.slice(2) : tableLines.slice(1)).map(parseRow);
            blocks.push({ kind: "table", headers, rows: dataRows });
            continue;
        }
        // Unordered list
        if (/^[ \t]*[-*+] /.test(line)) {
            const items = [];
            while (i < lines.length && /^[ \t]*[-*+] /.test(lines[i])) {
                items.push(lines[i].replace(/^[ \t]*[-*+] /, ""));
                i++;
            }
            blocks.push({ kind: "list", items });
            continue;
        }
        // Empty line
        if (line.trim() === "") {
            i++;
            continue;
        }
        // Paragraph — collect until blank or special line
        const paraLines = [];
        while (i < lines.length &&
            lines[i].trim() !== "" &&
            !/^(`{3,}|~{3,})/.test(lines[i]) &&
            !lines[i].trimStart().startsWith("|") &&
            !/^#{1,6}\s/.test(lines[i]) &&
            !/^[ \t]*[-*+] /.test(lines[i]) &&
            !/^[-*_]{3,}\s*$/.test(lines[i])) {
            paraLines.push(lines[i]);
            i++;
        }
        if (paraLines.length > 0) {
            blocks.push({ kind: "paragraph", text: paraLines.join("\n") });
        }
    }
    return blocks;
}
function MarkdownContent({ text }) {
    const blocks = parseMdBlocks(text);
    return ((0, jsx_runtime_1.jsx)("div", { className: "md-body", children: blocks.map((block, bi) => {
            const key = `b${bi}`;
            if (block.kind === "code") {
                return ((0, jsx_runtime_1.jsx)("pre", { className: "md-pre", children: (0, jsx_runtime_1.jsx)("code", { children: block.content }) }, key));
            }
            if (block.kind === "heading") {
                const Tag = HEADING_TAGS[Math.min(block.level - 1, 5)];
                return (0, jsx_runtime_1.jsx)(Tag, { className: `md-h md-h${block.level}`, children: renderInline(block.text, key) }, key);
            }
            if (block.kind === "hr") {
                return (0, jsx_runtime_1.jsx)("hr", { className: "md-hr" }, key);
            }
            if (block.kind === "table") {
                return ((0, jsx_runtime_1.jsx)("div", { className: "md-table-wrap", children: (0, jsx_runtime_1.jsxs)("table", { className: "md-table", children: [block.headers.length > 0 && ((0, jsx_runtime_1.jsx)("thead", { children: (0, jsx_runtime_1.jsx)("tr", { children: block.headers.map((h, hi) => (0, jsx_runtime_1.jsx)("th", { children: renderInline(h, `${key}-h${hi}`) }, hi)) }) })), (0, jsx_runtime_1.jsx)("tbody", { children: block.rows.map((row, ri) => ((0, jsx_runtime_1.jsx)("tr", { children: row.map((cell, ci) => (0, jsx_runtime_1.jsx)("td", { children: renderInline(cell, `${key}-r${ri}c${ci}`) }, ci)) }, ri))) })] }) }, key));
            }
            if (block.kind === "list") {
                return ((0, jsx_runtime_1.jsx)("ul", { className: "md-ul", children: block.items.map((item, ii) => (0, jsx_runtime_1.jsx)("li", { children: renderInline(item, `${key}-i${ii}`) }, ii)) }, key));
            }
            if (block.kind === "paragraph") {
                return (0, jsx_runtime_1.jsx)("p", { className: "md-p", children: renderInline(block.text, key) }, key);
            }
            return null;
        }) }));
}
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
const TranslatePopup_1 = require("./TranslatePopup");
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
    const isSpawn = it.toolName === "spawn_agent";
    if (isSpawn) {
        return ((0, jsx_runtime_1.jsxs)("div", { className: "toolrow spawn", children: ["\u2699 ", (0, jsx_runtime_1.jsx)("b", { children: "spawn_agent" }), "(", label, ")"] }));
    }
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
            return ((0, jsx_runtime_1.jsxs)(CopyBubble, { text: it.text, className: `bubble assistant ${it.finalized ? "final" : "streaming"}`, children: [(0, jsx_runtime_1.jsx)(MarkdownContent, { text: it.text }), !it.finalized && (0, jsx_runtime_1.jsx)("span", { className: "caret", children: "\u258C" })] }));
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
    const focusAgentRun = (0, store_1.useStore)((s) => s.focusAgentRun);
    const endRef = (0, react_1.useRef)(null);
    const timelineRef = (0, react_1.useRef)(null);
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
        // Defer scroll one rAF so any layout shift from pagination (e.g. "Load earlier"
        // button inserted at the top when a gate reprompt pushes totalPromptCount over
        // TIMELINE_PAGE_SIZE) is fully committed before we measure the scroll target. (BUG-146)
        const id = requestAnimationFrame(() => {
            endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
        });
        return () => cancelAnimationFrame(id);
    }, [timeline]);
    return ((0, jsx_runtime_1.jsxs)("div", { ref: timelineRef, className: `timeline ${activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId ? "timeline-agent-focused" : ""}`, children: [(0, jsx_runtime_1.jsx)(TranslatePopup_1.TranslatePopup, { containerRef: timelineRef }), activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId ? ((0, jsx_runtime_1.jsxs)("div", { className: "crumb ring", children: [(0, jsx_runtime_1.jsx)("button", { type: "button", className: "crumb-backbtn", onClick: backToMainRun, children: "\u2190 Back to main agent" }), (0, jsx_runtime_1.jsxs)("span", { className: "crumb-path", children: [(0, jsx_runtime_1.jsx)("b", { children: "main" }), " ", (0, jsx_runtime_1.jsx)("span", { style: { opacity: 0.5 }, children: "\u203A" }), " ", (0, jsx_runtime_1.jsx)("span", { className: "here", children: agentRuns.find(r => r.runId === activeAgentRunId)?.agentName ?? activeAgentRunId })] }), (0, jsx_runtime_1.jsxs)("span", { className: "crumb-path", style: { marginLeft: "auto", display: "inline-flex", alignItems: "center", gap: "6px" }, children: [(0, jsx_runtime_1.jsx)("span", { className: "pulse" }), " live child run"] })] })) : null, timeline.length === 0 && (0, jsx_runtime_1.jsx)("div", { className: "empty", children: "Select a project, choose your chat controls, and send a prompt to begin." }), hiddenPromptCount > 0 && ((0, jsx_runtime_1.jsxs)("button", { type: "button", className: "load-earlier-btn", onClick: () => setVisiblePromptCount((current) => Math.min(totalPromptCount, current + TIMELINE_PAGE_SIZE)), children: ["\u2191 Load earlier prompts (", hiddenPromptCount, ")"] })), timelineGroups.map((it) => ((0, jsx_runtime_1.jsx)(Item, { it: it }, it.id))), (!activeAgentRunId || activeAgentRunId === mainRunId) &&
                agentRuns
                    .filter((run) => run.status === "running" || run.status === "waiting_approval" || run.status === "waiting_question")
                    .map((run) => {
                    const lowerName = run.agentName.toLowerCase();
                    const roleClass = lowerName.includes("coder") ? "coder" : lowerName.includes("review") ? "reviewer" : lowerName.includes("test") ? "tester" : "";
                    const roleColor = roleClass === "coder" ? "var(--role-coder)" : roleClass === "reviewer" ? "var(--role-reviewer)" : roleClass === "tester" ? "var(--role-tester)" : "var(--text)";
                    const isWaiting = run.status === "waiting_approval" || run.status === "waiting_question";
                    const providerName = (0, store_1.providerLabel)(run.providerKey ?? "");
                    return ((0, jsx_runtime_1.jsxs)("div", { className: `abanner ${roleClass}`, children: [(0, jsx_runtime_1.jsx)("span", { className: `pulse ${isWaiting ? "amber" : ""}` }), (0, jsx_runtime_1.jsxs)("span", { children: [(0, jsx_runtime_1.jsx)("b", { style: { color: roleColor }, children: run.agentName }), " \u00B7 ", providerName, " \u00B7 ", run.status, run.agentStatus && (0, jsx_runtime_1.jsxs)("span", { style: { color: "var(--text-dim)" }, children: [" \u2014 ", run.agentStatus] })] }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "abanner-open-btn", onClick: () => void focusAgentRun(run.runId), children: "Open \u2197" })] }, run.runId));
                }), (0, jsx_runtime_1.jsx)("div", { ref: endRef })] }));
}
