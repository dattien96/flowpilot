"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.isChildRunFocused = isChildRunFocused;
exports.parseMentionRouting = parseMentionRouting;
exports.ChatInput = ChatInput;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const store_1 = require("@/state/store");
const normalizeImage_1 = require("@/lib/normalizeImage");
function CodexIcon() {
    return ((0, jsx_runtime_1.jsx)("svg", { width: "18", height: "18", viewBox: "-3 -3 30 30", fill: "currentColor", "aria-hidden": "true", children: (0, jsx_runtime_1.jsx)("path", { d: "M22.282 9.821a5.985 5.985 0 0 0-.516-4.911 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.985 5.985 0 0 0-3.998 2.9 6.046 6.046 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.514 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.99 5.99 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073zm-8.33 11.69a4.476 4.476 0 0 1-2.876-1.04l.141-.081 4.779-2.758a.796.796 0 0 0 .392-.68v-6.738l2.02 1.168a.07.07 0 0 1 .038.053v5.582a4.504 4.504 0 0 1-4.494 4.494zm-9.652-3.82a4.47 4.47 0 0 1-.535-3.014l.141.085 4.784 2.759a.77.77 0 0 0 .78 0l5.843-3.369v2.333a.08.08 0 0 1-.033.062L9.74 19.95a4.499 4.499 0 0 1-6.14-1.647zM2.34 7.896a4.485 4.485 0 0 1 2.366-1.973V11.6a.767.767 0 0 0 .388.677l5.815 3.354-2.02 1.168a.076.076 0 0 1-.071 0l-4.83-2.786A4.504 4.504 0 0 1 2.34 7.872zm16.597 3.855l-5.833-3.387L15.118 7.2a.076.076 0 0 1 .071 0l4.83 2.791a4.494 4.494 0 0 1-.677 8.105v-5.678a.79.79 0 0 0-.407-.667zm2.01-3.023l-.141-.085-4.774-2.782a.776.776 0 0 0-.784 0L9.41 9.23V6.898a.066.066 0 0 1 .028-.061l4.83-2.787a4.5 4.5 0 0 1 6.679 4.66zm-12.64 4.134l-2.02-1.164a.08.08 0 0 1-.038-.057V6.074a4.5 4.5 0 0 1 7.374-3.453l-.142.08-4.777 2.758a.795.795 0 0 0-.393.681zm1.097-2.365l2.602-1.5 2.607 1.5v2.999l-2.597 1.5-2.607-1.5Z" }) }));
}
function ClaudeIcon() {
    return ((0, jsx_runtime_1.jsxs)("svg", { width: "18", height: "18", viewBox: "0 0 24 24", fill: "currentColor", "aria-hidden": "true", children: [(0, jsx_runtime_1.jsx)("rect", { x: "10.75", y: "5.5", width: "2.5", height: "13", rx: "1.25" }), (0, jsx_runtime_1.jsx)("rect", { x: "10.75", y: "5.5", width: "2.5", height: "13", rx: "1.25", transform: "rotate(45 12 12)" }), (0, jsx_runtime_1.jsx)("rect", { x: "10.75", y: "5.5", width: "2.5", height: "13", rx: "1.25", transform: "rotate(90 12 12)" }), (0, jsx_runtime_1.jsx)("rect", { x: "10.75", y: "5.5", width: "2.5", height: "13", rx: "1.25", transform: "rotate(135 12 12)" })] }));
}
function GeminiIcon() {
    return ((0, jsx_runtime_1.jsx)("svg", { width: "18", height: "18", viewBox: "0 0 24 24", fill: "currentColor", "aria-hidden": "true", children: (0, jsx_runtime_1.jsx)("path", { d: "M12 2c0 5.52-4.48 10-10 10 5.52 0 10 4.48 10 10 0-5.52 4.48-10 10-10-5.52 0-10-4.48-10-10z" }) }));
}
function StopIcon() {
    return ((0, jsx_runtime_1.jsx)("svg", { width: "14", height: "14", viewBox: "0 0 14 14", fill: "none", stroke: "currentColor", strokeWidth: "2", strokeLinejoin: "round", "aria-hidden": "true", children: (0, jsx_runtime_1.jsx)("rect", { x: "2", y: "2", width: "10", height: "10", rx: "2" }) }));
}
function SwitchAccountIcon() {
    return ((0, jsx_runtime_1.jsxs)("svg", { width: "13", height: "13", viewBox: "0 0 14 14", fill: "none", stroke: "currentColor", strokeWidth: "1.5", strokeLinecap: "round", strokeLinejoin: "round", "aria-hidden": "true", children: [(0, jsx_runtime_1.jsx)("polyline", { points: "2,4 12,4 9,1" }), (0, jsx_runtime_1.jsx)("polyline", { points: "12,10 2,10 5,13" })] }));
}
const PROVIDER_CARDS = [
    { value: "codex", label: "Codex", icon: (0, jsx_runtime_1.jsx)(CodexIcon, {}) },
    { value: "claude", label: "Claude", icon: (0, jsx_runtime_1.jsx)(ClaudeIcon, {}) },
    { value: "gemini", label: "Gemini", icon: (0, jsx_runtime_1.jsx)(GeminiIcon, {}) },
];
// Providers whose runner adapters advertise the Vision capability (Task-052). Mirrors
// ProviderCapabilities.Vision in the Go runner; a follow-up should source this from the
// provider registration the renderer loads instead of hardcoding it here.
const VISION_PROVIDERS = new Set(["codex", "claude"]);
const REASONING_OPTIONS = [
    { value: "", label: "Default" },
    { value: "low", label: "Low" },
    { value: "medium", label: "Medium" },
    { value: "high", label: "High" },
];
function skillSourceLabel(source) {
    return source === "provider" ? "Account" : "Project";
}
function formatTokenCount(value) {
    if (value >= 1_000_000) {
        return `${(value / 1_000_000).toFixed(value >= 10_000_000 ? 0 : 1)}M`;
    }
    if (value >= 1_000) {
        return `${(value / 1_000).toFixed(value >= 10_000 ? 0 : 1)}k`;
    }
    return String(value);
}
function usageNumber(value) {
    return (0, jsx_runtime_1.jsx)("span", { className: "chat-usage-number", children: formatTokenCount(value) });
}
function usageSeparator() {
    return (0, jsx_runtime_1.jsx)("span", { className: "chat-usage-separator", children: " \u00B7 " });
}
function usageSummaryLine(provider, usage) {
    if (!provider)
        return null;
    if (!usage)
        return null;
    const last = usage.last;
    const total = usage.total;
    const windowSize = usage.modelContextWindow ?? null;
    const contextUsed = total?.totalTokens ?? last?.totalTokens ?? null;
    const parts = [];
    if (windowSize && contextUsed !== null) {
        const remaining = Math.max(windowSize - contextUsed, 0);
        parts.push((0, jsx_runtime_1.jsxs)("span", { children: ["Context ", usageNumber(contextUsed), " / ", usageNumber(windowSize), " used"] }, "context"));
        parts.push((0, jsx_runtime_1.jsxs)("span", { children: [usageNumber(remaining), " left"] }, "remaining"));
    }
    if (last) {
        parts.push((0, jsx_runtime_1.jsxs)("span", { children: ["Last turn ", usageNumber(last.totalTokens), " tokens"] }, "last-turn"));
        parts.push((0, jsx_runtime_1.jsxs)("span", { children: ["in ", usageNumber(last.inputTokens), " \u00B7 out ", usageNumber(last.outputTokens)] }, "in-out"));
    }
    if (parts.length === 0 && total) {
        parts.push((0, jsx_runtime_1.jsxs)("span", { children: ["Total ", usageNumber(total.totalTokens), " tokens"] }, "total"));
    }
    if (parts.length === 0)
        return null;
    return parts.map((part, index) => ((0, jsx_runtime_1.jsxs)("span", { children: [index > 0 ? usageSeparator() : null, part] }, index)));
}
// Build the backdrop children: plain strings interleaved with highlighted <mark> spans.
function buildBackdrop(text, tokens) {
    const sorted = [...tokens].sort((a, b) => a.start - b.start);
    const parts = [];
    let pos = 0;
    for (const token of sorted) {
        if (token.start > pos)
            parts.push(text.slice(pos, token.start));
        parts.push((0, jsx_runtime_1.jsx)("mark", { className: "skill-token-highlight", children: text.slice(token.start, token.end) }, `${token.name}-${token.start}`));
        pos = token.end;
    }
    if (pos < text.length)
        parts.push(text.slice(pos));
    return parts;
}
// Scan backwards from `cursor` to find an active slash command fragment.
// Returns the index of '/' and the query text, or null if none found.
// Valid triggers: '/' at position 0, or preceded by a space.
function findActiveSlash(text, cursor) {
    for (let i = cursor - 1; i >= 0; i--) {
        if (text[i] === "/") {
            if (i === 0 || text[i - 1] === " ") {
                return { index: i, query: text.slice(i + 1, cursor).toLowerCase() };
            }
            return null;
        }
        if (text[i] === " ")
            return null;
    }
    return null;
}
function isChildRunFocused(activeAgentRunId, mainRunId) {
    return Boolean(activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId);
}
function parseMentionRouting(text, agentRuns) {
    const trimmed = text.trim();
    const match = trimmed.match(/^@([A-Za-z0-9_-]+)\s*(.*)$/s);
    if (!match)
        return null;
    const agentName = match[1];
    const prompt = match[2].trim();
    const target = agentRuns.find((run) => run.agentName.toLowerCase() === agentName.toLowerCase());
    if (!target)
        return { kind: "missing", agentName };
    if (target.status === "running" || target.status === "waiting_approval" || target.status === "waiting_question") {
        return { kind: "busy", agentName, runId: target.runId, prompt };
    }
    return { kind: "focus", agentName, runId: target.runId, prompt };
}
// Chat composer + bottom controller strip. The controller keeps the current
// skill picker active while visually de-emphasizing the other workspace
// controls so the desktop layout reads like the mockup.
function ChatInput() {
    const skills = (0, store_1.useStore)((s) => s.skills);
    const projects = (0, store_1.useStore)((s) => s.projects);
    const sendPrompt = (0, store_1.useStore)((s) => s.sendPrompt);
    const status = (0, store_1.useStore)((s) => s.status);
    const runId = (0, store_1.useStore)((s) => s.runId);
    const chatMode = (0, store_1.useStore)((s) => s.chatMode);
    const launchMode = (0, store_1.useStore)((s) => s.launchMode);
    const supportedModels = (0, store_1.useStore)((s) => s.supportedModels);
    const selectedProjectId = (0, store_1.useStore)((s) => s.selectedProjectId);
    const selectedWorkflowId = (0, store_1.useStore)((s) => s.selectedWorkflowId);
    const selectedStepId = (0, store_1.useStore)((s) => s.selectedStepId);
    const selectedProvider = (0, store_1.useStore)((s) => s.selectedProvider);
    const selectedModel = (0, store_1.useStore)((s) => s.selectedModel);
    const reasoningEffort = (0, store_1.useStore)((s) => s.reasoningEffort);
    const yoloMode = (0, store_1.useStore)((s) => s.yoloMode);
    const loadSkills = (0, store_1.useStore)((s) => s.loadSkills);
    const selectProvider = (0, store_1.useStore)((s) => s.selectProvider);
    const setSelectedModel = (0, store_1.useStore)((s) => s.setSelectedModel);
    const setReasoningEffort = (0, store_1.useStore)((s) => s.setReasoningEffort);
    const setYoloMode = (0, store_1.useStore)((s) => s.setYoloMode);
    const pendingApproval = (0, store_1.useStore)((s) => s.pendingApproval);
    const pendingQuestion = (0, store_1.useStore)((s) => s.pendingQuestion);
    const latestTokenUsage = (0, store_1.useStore)((s) => s.latestTokenUsage);
    const stop = (0, store_1.useStore)((s) => s.stop);
    const timeline = (0, store_1.useStore)((s) => s.timeline);
    const providerAccounts = (0, store_1.useStore)((s) => s.providerAccounts);
    const agentRuns = (0, store_1.useStore)((s) => s.agentRuns);
    const activeAgentRunId = (0, store_1.useStore)((s) => s.activeAgentRunId);
    const mainRunId = (0, store_1.useStore)((s) => s.mainRunId ?? s.runId);
    const focusAgentRun = (0, store_1.useStore)((s) => s.focusAgentRun);
    const appendSystemMessage = (0, store_1.useStore)((s) => s.appendSystemMessage);
    const openAgentSpawnGuide = (0, store_1.useStore)((s) => s.openAgentSpawnGuide);
    const pendingAccountSwitch = (0, store_1.useStore)((s) => s.pendingAccountSwitch);
    const accountSwitchLoading = (0, store_1.useStore)((s) => s.accountSwitchLoading);
    const requestManualAccountSwitch = (0, store_1.useStore)((s) => s.requestManualAccountSwitch);
    const backToMainRun = (0, store_1.useStore)((s) => s.backToMainRun);
    const [text, setText] = (0, react_1.useState)("");
    const [selectedSkills, setSelectedSkills] = (0, react_1.useState)([]);
    const [pickerSortSelection, setPickerSortSelection] = (0, react_1.useState)([]);
    const [pickerSearch, setPickerSearch] = (0, react_1.useState)("");
    const [skillPickerOpen, setSkillPickerOpen] = (0, react_1.useState)(false);
    const [cursorPos, setCursorPos] = (0, react_1.useState)(0);
    const [slashDismissedIndex, setSlashDismissedIndex] = (0, react_1.useState)(null);
    const [skillTokens, setSkillTokens] = (0, react_1.useState)([]);
    const [pickerHighlightIndex, setPickerHighlightIndex] = (0, react_1.useState)(-1);
    const [controllerExpanded, setControllerExpanded] = (0, react_1.useState)(true);
    const [attachments, setAttachments] = (0, react_1.useState)([]);
    const [attachError, setAttachError] = (0, react_1.useState)(null);
    const [previewAtt, setPreviewAtt] = (0, react_1.useState)(null);
    const [displayedTokenUsage, setDisplayedTokenUsage] = (0, react_1.useState)(undefined);
    const fileInputRef = (0, react_1.useRef)(null);
    const searchInputRef = (0, react_1.useRef)(null);
    const textAreaRef = (0, react_1.useRef)(null);
    const rootRef = (0, react_1.useRef)(null);
    const wasPickerVisibleRef = (0, react_1.useRef)(false);
    const isChatMode = chatMode === "normal_chat";
    // Lock provider once any turn has been sent in the current chat session.
    const providerLocked = isChatMode && timeline.length > 0;
    const supportsVision = !!selectedProvider && VISION_PROVIDERS.has(selectedProvider);
    const hasSelectedProject = !!selectedProjectId;
    const selectedProjectPath = (0, react_1.useMemo)(() => projects.find((project) => project.id === selectedProjectId)?.path, [projects, selectedProjectId]);
    const availableModels = (0, react_1.useMemo)(() => supportedModels
        .filter((model) => model.providerKey === selectedProvider && model.isEnabled)
        .sort((a, b) => a.sortOrder - b.sortOrder), [selectedProvider, supportedModels]);
    const slashFragment = (0, react_1.useMemo)(() => {
        if (!isChatMode)
            return null;
        const frag = findActiveSlash(text, cursorPos);
        if (frag !== null && frag.index === slashDismissedIndex)
            return null;
        return frag;
    }, [isChatMode, text, cursorPos, slashDismissedIndex]);
    const slashQuery = slashFragment?.query ?? null;
    // Slash sub-commands are namespaced by the command letter after "/": "/a…" (or "/agent")
    // opens the spawn-agent UI, "/s…" (or "/skill") opens the skill picker. A bare "/" (or any
    // other leading letter) opens nothing — a command letter is required so "/" no longer pops
    // the skill UI on its own (BUG-134). The remainder after "/s" is the skill search term.
    const slashCommand = slashFragment === null
        ? null
        : slashFragment.query[0] === "a"
            ? "agent"
            : slashFragment.query[0] === "s"
                ? "skill"
                : null;
    const showAgentCommand = isChatMode && !!selectedProvider && slashCommand === "agent";
    const showPicker = isChatMode && !!selectedProvider && (skillPickerOpen || slashCommand === "skill");
    const totalSkills = skills.length;
    const filtered = (0, react_1.useMemo)(() => {
        if (!showPicker)
            return [];
        const query = pickerSearch.trim().toLowerCase();
        const matchingSkills = query.length === 0
            ? skills
            : skills.filter((s) => s.name.toLowerCase().includes(query));
        const sortedSkills = [...matchingSkills].sort((left, right) => left.name.localeCompare(right.name));
        const selected = sortedSkills.filter((skill) => pickerSortSelection.includes(skill.name));
        const remaining = sortedSkills.filter((skill) => !pickerSortSelection.includes(skill.name));
        return [...selected, ...remaining];
    }, [showPicker, pickerSearch, skills, pickerSortSelection]);
    (0, react_1.useEffect)(() => {
        if (!isChatMode) {
            setSkillPickerOpen(false);
            setSelectedSkills([]);
            return;
        }
        if (!selectedProvider) {
            setSkillPickerOpen(false);
            setSelectedSkills([]);
            return;
        }
        setSkillPickerOpen(false);
        setSelectedSkills([]);
        void loadSkills(selectedProvider, selectedProjectPath);
    }, [isChatMode, loadSkills, selectedProjectPath, selectedProvider]);
    (0, react_1.useEffect)(() => {
        if (!isChatMode)
            return;
        if (availableModels.length === 0)
            return;
        if (!selectedModel)
            return;
        if (!availableModels.some((model) => model.modelId === selectedModel)) {
            setSelectedModel(undefined);
        }
    }, [availableModels, isChatMode, selectedModel, setSelectedModel]);
    (0, react_1.useEffect)(() => {
        if (isChatMode) {
            setControllerExpanded(true);
        }
    }, [isChatMode, runId]);
    // Drop pending attachments when leaving chat mode or when the selected provider
    // cannot accept images (Task-052) — they would have nowhere valid to go.
    (0, react_1.useEffect)(() => {
        if (!isChatMode || !supportsVision) {
            setAttachments([]);
            setAttachError(null);
        }
    }, [isChatMode, supportsVision]);
    (0, react_1.useEffect)(() => {
        if (!previewAtt)
            return;
        const onKey = (e) => { if (e.key === "Escape")
            setPreviewAtt(null); };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, [previewAtt]);
    (0, react_1.useEffect)(() => {
        setSelectedSkills((prev) => prev.filter((name) => skills.some((skill) => skill.name === name)));
    }, [skills]);
    (0, react_1.useEffect)(() => {
        if (showPicker && !wasPickerVisibleRef.current) {
            setPickerSortSelection(selectedSkills);
            searchInputRef.current?.focus();
        }
        wasPickerVisibleRef.current = showPicker;
    }, [showPicker, selectedSkills]);
    // Keep pickerSearch in sync with the slash query so typing /foo in the textarea
    // still drives the in-picker filter in real time.
    (0, react_1.useEffect)(() => {
        if (slashCommand !== "skill" || slashQuery === null)
            return;
        // The skill picker lives under the "/s" namespace, so the leading "s" (or the full
        // "skill" word) is the command, not a search term; everything after it filters the list.
        const term = slashQuery === "s" || slashQuery === "skill" ? "" : slashQuery.slice(1);
        setPickerSearch(term);
    }, [slashQuery, slashCommand]);
    // Clear the search box whenever the picker is dismissed.
    (0, react_1.useEffect)(() => {
        if (!showPicker)
            setPickerSearch("");
    }, [showPicker]);
    // Reset keyboard highlight whenever the filtered list changes.
    (0, react_1.useEffect)(() => {
        setPickerHighlightIndex(-1);
    }, [pickerSearch]);
    (0, react_1.useEffect)(() => {
        if (!showPicker)
            return;
        const onPointerDown = (event) => {
            const root = rootRef.current;
            if (!root || root.contains(event.target))
                return;
            setSkillPickerOpen(false);
            setCursorPos(0);
        };
        window.addEventListener("pointerdown", onPointerDown);
        return () => window.removeEventListener("pointerdown", onPointerDown);
    }, [showPicker]);
    (0, react_1.useEffect)(() => {
        if (latestTokenUsage) {
            setDisplayedTokenUsage(latestTokenUsage);
        }
    }, [latestTokenUsage]);
    (0, react_1.useEffect)(() => {
        setDisplayedTokenUsage(undefined);
    }, [selectedProvider]);
    const hasBetterAccount = (0, react_1.useMemo)(() => !!selectedProvider &&
        providerAccounts.some((a) => a.providerKey === selectedProvider && a.authStatus === "connected" && !a.isActive), [providerAccounts, selectedProvider]);
    const childRunFocused = isChildRunFocused(activeAgentRunId, mainRunId);
    const focusedAgentName = childRunFocused
        ? agentRuns.find((run) => run.runId === activeAgentRunId)?.agentName ?? activeAgentRunId
        : undefined;
    const connectedProviders = (0, react_1.useMemo)(() => new Set(providerAccounts.filter((account) => account.authStatus === "connected").map((account) => account.providerKey)), [providerAccounts]);
    const selectedProviderConnected = !!selectedProvider && connectedProviders.has(selectedProvider);
    const blocked = status === "running" || status === "waiting_approval" || status === "waiting_question";
    // A running child spawned with wait=true blocks the main run even when the main has no turn
    // of its own in flight (e.g. a UI wait=true spawn) — the send button must reflect that (BUG-133).
    const hasBlockingChild = agentRuns.some((r) => r.waitForResult && (r.status === "running" || r.status === "waiting_approval" || r.status === "waiting_question"));
    const usageLine = (0, react_1.useMemo)(() => usageSummaryLine(selectedProvider, displayedTokenUsage), [displayedTokenUsage, selectedProvider]);
    const canSend = isChatMode
        ? hasSelectedProject && selectedProviderConnected && !blocked && !hasBlockingChild && !childRunFocused && text.trim().length > 0 && !showPicker && !showAgentCommand
        : hasSelectedProject &&
            (launchMode === "workflow" ? !!selectedWorkflowId : !!selectedStepId) &&
            !blocked &&
            text.trim().length > 0;
    // Keep the skill picker multi-select active; the controller strip visually
    // downplays the other options but still reflects the current runtime state.
    const pickSkill = (name) => {
        setSelectedSkills((prev) => (prev.includes(name) ? prev : [...prev, name]));
        if (slashFragment !== null) {
            // Slash/command mode: replace the /query fragment in the prompt text and
            // close the picker (single-select per slash token).
            const before = text.slice(0, slashFragment.index);
            const after = text.slice(cursorPos);
            const newText = before + name + after;
            const newCursor = slashFragment.index + name.length;
            setText(newText);
            setCursorPos(newCursor);
            // Record token position so the backdrop can highlight it.
            const tokenStart = slashFragment.index;
            setSkillTokens((prev) => [...prev, { name, start: tokenStart, end: tokenStart + name.length }]);
            setTimeout(() => {
                if (textAreaRef.current) {
                    textAreaRef.current.selectionStart = newCursor;
                    textAreaRef.current.selectionEnd = newCursor;
                    textAreaRef.current.focus();
                }
            }, 0);
            setSkillPickerOpen(false);
            setPickerHighlightIndex(-1);
            setSlashDismissedIndex(null);
        }
        // Touch/button mode (slashFragment === null): keep picker open so the user
        // can select multiple skills before dismissing manually.
    };
    const removeSkill = (name) => {
        setSelectedSkills((prev) => prev.filter((s) => s !== name));
    };
    // "/a" command: strip the slash fragment from the prompt and open the spawn-agent
    // panel (the same UI as the right sidebar), then restore the caret. (Task-087)
    const triggerAgentSlash = () => {
        if (slashFragment === null)
            return;
        const before = text.slice(0, slashFragment.index);
        const after = text.slice(cursorPos);
        const newCursor = slashFragment.index;
        setText(before + after);
        setCursorPos(newCursor);
        setSlashDismissedIndex(null);
        setSkillPickerOpen(false);
        setTimeout(() => {
            if (textAreaRef.current) {
                textAreaRef.current.selectionStart = newCursor;
                textAreaRef.current.selectionEnd = newCursor;
                textAreaRef.current.focus();
            }
        }, 0);
        openAgentSpawnGuide();
    };
    const onPaste = (e) => {
        if (!supportsVision || !isChatMode || blocked)
            return;
        const items = Array.from(e.clipboardData?.items ?? []).filter((item) => item.kind === "file" && item.type.startsWith("image/"));
        if (items.length === 0)
            return;
        e.preventDefault();
        const dt = new DataTransfer();
        for (const item of items) {
            const file = item.getAsFile();
            if (file)
                dt.items.add(file);
        }
        if (dt.files.length > 0)
            void onPickFiles(dt.files);
    };
    const onPickFiles = async (fileList) => {
        if (!fileList || fileList.length === 0)
            return;
        setAttachError(null);
        const files = Array.from(fileList);
        const room = normalizeImage_1.MAX_ATTACHMENTS - attachments.length;
        if (room <= 0) {
            setAttachError(`Up to ${normalizeImage_1.MAX_ATTACHMENTS} images per message.`);
            return;
        }
        const accepted = files.slice(0, room);
        const next = [];
        for (const file of accepted) {
            try {
                next.push(await (0, normalizeImage_1.normalizeImage)(file));
            }
            catch (err) {
                setAttachError(err instanceof normalizeImage_1.ImageNormalizeError ? err.message : `Could not attach ${file.name}.`);
            }
        }
        if (next.length > 0)
            setAttachments((prev) => [...prev, ...next]);
        if (files.length > room)
            setAttachError(`Up to ${normalizeImage_1.MAX_ATTACHMENTS} images per message.`);
    };
    const removeAttachment = (id) => {
        setAttachments((prev) => prev.filter((a) => a.id !== id));
    };
    const clearComposer = () => {
        setText("");
        setSelectedSkills([]);
        setSkillTokens([]);
        setAttachments([]);
        setAttachError(null);
        setSkillPickerOpen(false);
    };
    const send = async () => {
        if (!canSend)
            return;
        if (childRunFocused) {
            appendSystemMessage("Child transcript is read-only. Return to the main chat to send prompts.");
            return;
        }
        const trimmed = text.trim();
        const routed = parseMentionRouting(trimmed, agentRuns);
        if (routed) {
            if (routed.kind === "missing") {
                appendSystemMessage(`No child run named @${routed.agentName}. Open Agents and spawn it first.`);
                openAgentSpawnGuide(routed.agentName);
                return;
            }
            if (routed.kind === "busy") {
                await store_1.useStore.getState().injectAgentFeedback(routed.runId, routed.prompt || trimmed);
                appendSystemMessage(`Queued feedback for @${routed.agentName}. It will be picked up when the child is safe to continue.`);
                return;
            }
            await focusAgentRun(routed.runId);
            if (routed.prompt.length === 0) {
                appendSystemMessage(`Focused @${routed.agentName}. Add a prompt to send work to this child run.`);
                return;
            }
            const routedAttachments = isChatMode && attachments.length > 0 ? attachments.map(normalizeImage_1.toWire) : undefined;
            clearComposer();
            await sendPrompt(routed.prompt, isChatMode ? selectedSkills : undefined, routedAttachments);
            return;
        }
        const wireAttachments = isChatMode && attachments.length > 0 ? attachments.map(normalizeImage_1.toWire) : undefined;
        clearComposer();
        await sendPrompt(trimmed, isChatMode ? selectedSkills : undefined, wireAttachments);
    };
    const onKeyDown = (e) => {
        if (showAgentCommand) {
            if (e.key === "Enter") {
                e.preventDefault();
                triggerAgentSlash();
                return;
            }
            if (e.key === "Escape") {
                e.preventDefault();
                if (slashFragment !== null)
                    setSlashDismissedIndex(slashFragment.index);
                return;
            }
        }
        if (showPicker && slashFragment !== null) {
            if (e.key === "ArrowDown") {
                e.preventDefault();
                setPickerHighlightIndex((prev) => Math.min(prev + 1, filtered.length - 1));
                return;
            }
            if (e.key === "ArrowUp") {
                e.preventDefault();
                setPickerHighlightIndex((prev) => Math.max(prev - 1, 0));
                return;
            }
            if (e.key === "Enter" && pickerHighlightIndex >= 0) {
                e.preventDefault();
                const highlighted = filtered[pickerHighlightIndex];
                if (highlighted) {
                    if (selectedSkills.includes(highlighted.name))
                        removeSkill(highlighted.name);
                    else
                        pickSkill(highlighted.name);
                }
                return;
            }
            if (e.key === "Escape") {
                e.preventDefault();
                if (slashFragment !== null)
                    setSlashDismissedIndex(slashFragment.index);
                setSkillPickerOpen(false);
                setPickerHighlightIndex(-1);
                return;
            }
        }
        if (e.key === "Enter" && !e.shiftKey && !showPicker) {
            e.preventDefault();
            void send();
        }
    };
    const placeholder = hasBlockingChild && !blocked
        ? "Waiting for a sub-agent (wait=true) to finish…"
        : blocked
            ? "Waiting for the current turn..."
            : !hasSelectedProject
                ? "Select a project first."
                : isChatMode
                    ? childRunFocused
                        ? "Child transcript is read-only."
                        : selectedProvider
                            ? "Type a message. Use /s for skills, /a to spawn an agent, @ to message an agent."
                            : "Select a provider first."
                    : launchMode === "workflow"
                        ? selectedWorkflowId
                            ? "Type a message. Enter to send."
                            : "Select a workflow first."
                        : selectedStepId
                            ? "Type a message. Enter to send."
                            : "Select a step first.";
    return ((0, jsx_runtime_1.jsxs)("div", { ref: rootRef, className: `chat-input ${!hasSelectedProject ? "chat-input-locked" : ""}`, children: [isChatMode && ((0, jsx_runtime_1.jsx)("div", { className: `chat-controller ${controllerExpanded ? "expanded" : "collapsed"}`, children: controllerExpanded && ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)("div", { className: "chat-controller-top", children: [(0, jsx_runtime_1.jsxs)("div", { className: "chat-controller-skill-strip", children: [(0, jsx_runtime_1.jsx)("span", { className: "chat-controller-label", children: "Skills" }), (0, jsx_runtime_1.jsxs)("button", { type: "button", className: `skill-select-button ${showPicker ? "active" : ""}`, onClick: () => setSkillPickerOpen((current) => !current), "aria-expanded": showPicker, disabled: !selectedProvider || blocked, "aria-label": selectedProvider
                                                ? `Selected skills ${selectedSkills.length} of ${totalSkills}`
                                                : "Select skills", children: [(0, jsx_runtime_1.jsxs)("span", { className: "skill-select-icon", "aria-hidden": "true", children: [(0, jsx_runtime_1.jsx)("span", {}), (0, jsx_runtime_1.jsx)("span", {}), (0, jsx_runtime_1.jsx)("span", {})] }), (0, jsx_runtime_1.jsx)("span", { className: "skill-select-text", children: !selectedProvider
                                                        ? "Select provider first"
                                                        : `Selected skills ${selectedSkills.length}/${totalSkills}` }), (0, jsx_runtime_1.jsx)("span", { className: "skill-select-caret", "aria-hidden": "true", children: "\u25BE" })] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "chat-controller-head", children: [isChatMode && hasBetterAccount && ((0, jsx_runtime_1.jsxs)("button", { type: "button", className: "acc-switch-btn", onClick: requestManualAccountSwitch, disabled: blocked || !!pendingAccountSwitch || accountSwitchLoading, title: "Switch to a better account for this provider", "aria-label": "Switch to a better account", children: [(0, jsx_runtime_1.jsx)(SwitchAccountIcon, {}), (0, jsx_runtime_1.jsx)("span", { children: "Switch acc" })] })), (0, jsx_runtime_1.jsxs)("div", { className: "chat-controller-switch chat-controller-switch-top", children: [(0, jsx_runtime_1.jsx)("span", { className: "chat-controller-label", children: "YOLO" }), (0, jsx_runtime_1.jsxs)("button", { type: "button", role: "switch", "aria-checked": yoloMode, className: `yolo-toggle ${yoloMode ? "active" : ""}`, onClick: () => setYoloMode(!yoloMode), disabled: blocked, children: [(0, jsx_runtime_1.jsx)("span", { className: "yolo-toggle-track", "aria-hidden": "true", children: (0, jsx_runtime_1.jsx)("span", { className: "yolo-toggle-thumb" }) }), (0, jsx_runtime_1.jsx)("span", { className: "yolo-toggle-label", children: yoloMode ? "On" : "Off" })] })] }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "chat-controller-toggle", onClick: () => setControllerExpanded(false), "aria-label": "Collapse chat controls", children: (0, jsx_runtime_1.jsx)("span", { "aria-hidden": "true", children: "\u25BE" }) })] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "chat-controller-grid", children: [(0, jsx_runtime_1.jsxs)("div", { className: `provider-picker${providerLocked ? " provider-picker-locked" : ""}`, children: [(0, jsx_runtime_1.jsx)("span", { className: "chat-controller-label", children: "Provider" }), (0, jsx_runtime_1.jsx)("div", { className: "provider-chips", children: PROVIDER_CARDS.map((p) => ((0, jsx_runtime_1.jsxs)("button", { type: "button", className: `provider-chip provider-chip-${p.value}${selectedProvider === p.value ? " provider-chip-selected" : ""}${connectedProviders.has(p.value) ? "" : " provider-chip-unavailable"}`, onClick: () => selectProvider(selectedProvider === p.value ? undefined : p.value), disabled: blocked || providerLocked || !connectedProviders.has(p.value), "aria-pressed": selectedProvider === p.value, "aria-label": p.label, title: !connectedProviders.has(p.value)
                                                    ? `${p.label} is unavailable until an account is connected`
                                                    : providerLocked
                                                        ? "Start a new chat to change provider"
                                                        : p.label, children: [(0, jsx_runtime_1.jsx)("span", { className: "provider-chip-icon", children: p.icon }), (0, jsx_runtime_1.jsx)("span", { className: "provider-chip-name", children: p.label })] }, p.value))) })] }), (0, jsx_runtime_1.jsxs)("label", { className: "chat-controller-field muted", children: [(0, jsx_runtime_1.jsx)("span", { children: "Model" }), availableModels.length > 0 ? ((0, jsx_runtime_1.jsxs)("select", { value: selectedModel ?? "", onChange: (e) => setSelectedModel(e.target.value || undefined), disabled: blocked, children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "Default" }), availableModels.map((model) => ((0, jsx_runtime_1.jsx)("option", { value: model.modelId, children: model.displayName }, model.id)))] })) : ((0, jsx_runtime_1.jsx)("input", { type: "text", value: selectedModel ?? "", onChange: (e) => setSelectedModel(e.target.value || undefined), placeholder: "Default", disabled: blocked }))] }), (0, jsx_runtime_1.jsxs)("label", { className: "chat-controller-field muted", children: [(0, jsx_runtime_1.jsx)("span", { children: "Reasoning" }), (0, jsx_runtime_1.jsx)("select", { value: reasoningEffort ?? "", onChange: (e) => setReasoningEffort(e.target.value || undefined), disabled: blocked, children: REASONING_OPTIONS.map((option) => ((0, jsx_runtime_1.jsx)("option", { value: option.value, children: option.label }, option.value))) })] })] })] })) })), showPicker && ((0, jsx_runtime_1.jsxs)("div", { className: "skill-picker", children: [(0, jsx_runtime_1.jsxs)("div", { className: "skill-picker-head", children: [(0, jsx_runtime_1.jsxs)("div", { className: "skill-picker-head-top", children: [(0, jsx_runtime_1.jsx)("span", { children: "Skills \u00B7 pick one or more" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "skill-picker-close", onClick: () => {
                                            if (slashFragment !== null)
                                                setSlashDismissedIndex(slashFragment.index);
                                            setSkillPickerOpen(false);
                                            setPickerHighlightIndex(-1);
                                        }, "aria-label": "Close skills picker", children: "\u00D7" })] }), (0, jsx_runtime_1.jsx)("input", { ref: searchInputRef, type: "text", className: "skill-picker-search", placeholder: "Search skills\u2026", value: pickerSearch, onChange: (e) => setPickerSearch(e.target.value), onKeyDown: (e) => {
                                    if (e.key === "ArrowDown") {
                                        e.preventDefault();
                                        setPickerHighlightIndex((prev) => Math.min(prev + 1, filtered.length - 1));
                                    }
                                    else if (e.key === "ArrowUp") {
                                        e.preventDefault();
                                        setPickerHighlightIndex((prev) => Math.max(prev - 1, 0));
                                    }
                                    else if (e.key === "Enter" && pickerHighlightIndex >= 0) {
                                        e.preventDefault();
                                        const highlighted = filtered[pickerHighlightIndex];
                                        if (highlighted) {
                                            if (selectedSkills.includes(highlighted.name))
                                                removeSkill(highlighted.name);
                                            else
                                                pickSkill(highlighted.name);
                                        }
                                    }
                                    else if (e.key === "Escape") {
                                        e.preventDefault();
                                        if (slashFragment !== null)
                                            setSlashDismissedIndex(slashFragment.index);
                                        setSkillPickerOpen(false);
                                        setPickerHighlightIndex(-1);
                                        const restoreTo = cursorPos;
                                        setTimeout(() => {
                                            if (textAreaRef.current) {
                                                textAreaRef.current.focus();
                                                textAreaRef.current.selectionStart = restoreTo;
                                                textAreaRef.current.selectionEnd = restoreTo;
                                            }
                                        }, 0);
                                    }
                                }, "aria-label": "Search skills" })] }), filtered.length === 0 && (0, jsx_runtime_1.jsx)("div", { className: "skill-empty", children: "No matching skill" }), filtered.map((s, idx) => {
                        const active = selectedSkills.includes(s.name);
                        const highlighted = idx === pickerHighlightIndex;
                        return ((0, jsx_runtime_1.jsxs)("button", { type: "button", className: `skill-item ${active ? "skill-item-active" : ""} ${highlighted ? "skill-item-highlighted" : ""}`, onClick: () => (active ? removeSkill(s.name) : pickSkill(s.name)), children: [(0, jsx_runtime_1.jsx)("span", { className: "skill-mark", children: active ? "☑" : "☐" }), (0, jsx_runtime_1.jsxs)("span", { className: "skill-copy", children: [(0, jsx_runtime_1.jsxs)("span", { className: `skill-name ${active ? "skill-name-active" : "skill-name-idle"}`, children: ["/", s.name] }), s.description && (0, jsx_runtime_1.jsx)("span", { className: "skill-desc", title: s.description, children: s.description })] }), (0, jsx_runtime_1.jsx)("span", { className: `skill-src src-${s.source}`, children: skillSourceLabel(s.source) })] }, s.name));
                    })] })), showAgentCommand && ((0, jsx_runtime_1.jsxs)("div", { className: "skill-picker", children: [(0, jsx_runtime_1.jsx)("div", { className: "skill-picker-head", children: (0, jsx_runtime_1.jsxs)("div", { className: "skill-picker-head-top", children: [(0, jsx_runtime_1.jsx)("span", { children: "Agent command" }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "skill-picker-close", onClick: () => { if (slashFragment !== null)
                                        setSlashDismissedIndex(slashFragment.index); }, "aria-label": "Close agent command", children: "\u00D7" })] }) }), (0, jsx_runtime_1.jsxs)("button", { type: "button", className: "skill-item skill-item-highlighted", onMouseDown: (e) => { e.preventDefault(); triggerAgentSlash(); }, children: [(0, jsx_runtime_1.jsx)("span", { className: "skill-mark", children: "\uD83E\uDD16" }), (0, jsx_runtime_1.jsxs)("span", { className: "skill-copy", children: [(0, jsx_runtime_1.jsx)("span", { className: "skill-name skill-name-idle", children: "/a \u00B7 Spawn sub-agent" }), (0, jsx_runtime_1.jsx)("span", { className: "skill-desc", children: "Open the spawn-agent panel (same as the right sidebar). Press Enter." })] })] })] })), isChatMode && supportsVision && attachments.length > 0 && ((0, jsx_runtime_1.jsx)("div", { className: "chat-attachments", "aria-label": "Pending image attachments", children: attachments.map((att) => ((0, jsx_runtime_1.jsxs)("div", { className: "chat-attachment-chip", role: "button", tabIndex: 0, onClick: () => setPreviewAtt(att), onKeyDown: (e) => { if (e.key === "Enter" || e.key === " ")
                        setPreviewAtt(att); }, "aria-label": `Preview ${att.originalName}`, title: "Click to preview", children: [(0, jsx_runtime_1.jsx)("img", { className: "chat-attachment-thumb", src: att.previewUrl, alt: att.originalName }), (0, jsx_runtime_1.jsx)("span", { className: "chat-attachment-name", title: att.originalName, children: att.originalName }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "chat-attachment-remove", onClick: (e) => { e.stopPropagation(); removeAttachment(att.id); }, disabled: blocked, "aria-label": `Remove ${att.originalName}`, children: "\u00D7" })] }, att.id))) })), isChatMode && attachError && ((0, jsx_runtime_1.jsx)("div", { className: "chat-attachment-error", role: "alert", children: attachError })), isChatMode && childRunFocused && ((0, jsx_runtime_1.jsxs)("div", { className: "ctxbar ring", children: ["\u21B3 Viewing child agent ", (0, jsx_runtime_1.jsx)("b", { children: focusedAgentName }), " \u00B7 transcript only"] })), (0, jsx_runtime_1.jsxs)("div", { className: "input-bar", children: [isChatMode && !controllerExpanded && !childRunFocused && ((0, jsx_runtime_1.jsx)("button", { type: "button", className: "chat-controller-toggle chat-controller-toggle-inline", onClick: () => setControllerExpanded(true), "aria-label": "Expand chat controls", children: (0, jsx_runtime_1.jsxs)("span", { className: "chat-controller-menu-icon", "aria-hidden": "true", children: [(0, jsx_runtime_1.jsx)("span", {}), (0, jsx_runtime_1.jsx)("span", {}), (0, jsx_runtime_1.jsx)("span", {})] }) })), isChatMode && !childRunFocused && ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)("input", { ref: fileInputRef, type: "file", accept: normalizeImage_1.ACCEPT_ATTR, multiple: true, hidden: true, onChange: (e) => {
                                    void onPickFiles(e.target.files);
                                    e.target.value = ""; // allow re-picking the same file
                                } }), (0, jsx_runtime_1.jsxs)("button", { type: "button", className: "attach-btn", onClick: () => fileInputRef.current?.click(), disabled: !supportsVision || blocked || attachments.length >= normalizeImage_1.MAX_ATTACHMENTS, "aria-label": supportsVision
                                    ? "Attach image"
                                    : "Image attachments are not supported by the selected provider", title: supportsVision ? "Attach image (or paste with Ctrl+V)" : "Selected provider does not support images", children: [(0, jsx_runtime_1.jsx)("span", { "aria-hidden": "true", children: "\uD83D\uDCCE" }), attachments.length > 0 && ((0, jsx_runtime_1.jsx)("span", { className: "attach-badge", "aria-label": `${attachments.length} image${attachments.length > 1 ? "s" : ""} attached`, children: attachments.length }))] })] })), childRunFocused ? ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)("div", { className: "text-area-wrapper", children: (0, jsx_runtime_1.jsx)("div", { className: "input-note", children: "Return to the main chat to send prompts or use @agent routing." }) }), blocked && ((0, jsx_runtime_1.jsx)("button", { className: "btn send-btn send-btn-stop", onClick: () => void stop(), "aria-label": "Stop child agent", children: (0, jsx_runtime_1.jsx)(StopIcon, {}) })), (0, jsx_runtime_1.jsx)("button", { className: "btn send-btn", onClick: backToMainRun, children: "Main" })] })) : ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)("div", { className: `text-area-wrapper${isChatMode && skillTokens.length > 0 ? " has-highlights" : ""}`, children: [isChatMode && skillTokens.length > 0 && ((0, jsx_runtime_1.jsx)("div", { className: "text-area-backdrop", "aria-hidden": "true", children: buildBackdrop(text, skillTokens) })), (0, jsx_runtime_1.jsx)("textarea", { ref: textAreaRef, className: "text-area", rows: 2, placeholder: placeholder, value: text, onChange: (e) => {
                                            const newText = e.target.value;
                                            const newCursor = e.target.selectionStart ?? 0;
                                            setText(newText);
                                            setCursorPos(newCursor);
                                            if (slashDismissedIndex !== null && newText[slashDismissedIndex] !== "/") {
                                                setSlashDismissedIndex(null);
                                            }
                                            setSkillTokens((prev) => prev.filter((t) => newText.slice(t.start, t.end) === t.name));
                                        }, onKeyDown: onKeyDown, onPaste: onPaste, onSelect: (e) => setCursorPos(e.target.selectionStart ?? 0), onPointerDown: () => {
                                            if (skillPickerOpen)
                                                setSkillPickerOpen(false);
                                        } })] }), blocked ? ((0, jsx_runtime_1.jsx)("button", { className: "btn send-btn send-btn-stop", onClick: () => void stop(), "aria-label": "Stop AI", children: (0, jsx_runtime_1.jsx)(StopIcon, {}) })) : ((0, jsx_runtime_1.jsx)("button", { className: "btn btn-primary send-btn", onClick: send, disabled: !canSend, children: "Send" }))] }))] }), isChatMode && usageLine && ((0, jsx_runtime_1.jsx)("div", { className: "chat-usage-line", "aria-live": "polite", children: usageLine })), (pendingApproval || pendingQuestion) && ((0, jsx_runtime_1.jsx)("div", { className: "input-note", children: "Action required above before continuing." })), previewAtt && ((0, jsx_runtime_1.jsx)("div", { className: "attach-preview-overlay", role: "dialog", "aria-modal": "true", "aria-label": `Preview: ${previewAtt.originalName}`, onClick: () => setPreviewAtt(null), children: (0, jsx_runtime_1.jsxs)("div", { className: "attach-preview-box", onClick: (e) => e.stopPropagation(), children: [(0, jsx_runtime_1.jsxs)("div", { className: "attach-preview-header", children: [(0, jsx_runtime_1.jsx)("span", { className: "attach-preview-name", children: previewAtt.originalName }), (0, jsx_runtime_1.jsxs)("span", { className: "attach-preview-meta", children: [previewAtt.width && previewAtt.height ? `${previewAtt.width}×${previewAtt.height} · ` : "", (previewAtt.sizeBytes / 1024).toFixed(0), " KB"] }), (0, jsx_runtime_1.jsx)("button", { type: "button", className: "attach-preview-close", onClick: () => setPreviewAtt(null), "aria-label": "Close preview", children: "\u00D7" })] }), (0, jsx_runtime_1.jsx)("img", { className: "attach-preview-img", src: previewAtt.previewUrl, alt: previewAtt.originalName })] }) })), !hasSelectedProject && ((0, jsx_runtime_1.jsx)("div", { className: `chat-input-guard ${isChatMode ? "" : "chat-input-guard-compact"}`.trim(), "aria-live": "polite", children: (0, jsx_runtime_1.jsxs)("div", { className: `chat-input-guard-card ${isChatMode ? "" : "chat-input-guard-card-compact"}`.trim(), children: [(0, jsx_runtime_1.jsx)("div", { className: "chat-input-guard-title", children: "Select a project first" }), (0, jsx_runtime_1.jsx)("div", { className: "chat-input-guard-copy", children: isChatMode
                                ? "Choose a project before using chat, skills, or send."
                                : "Choose a project before continuing in workflow mode." })] }) }))] }));
}
