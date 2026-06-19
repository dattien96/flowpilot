import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useStore } from "@/state/store";
import type { ProviderKey, TokenUsageSnapshot } from "@/types/contract";
import {
  ACCEPT_ATTR,
  MAX_ATTACHMENTS,
  ImageNormalizeError,
  normalizeImage,
  toWire,
  type PendingAttachment,
} from "@/lib/normalizeImage";

function CodexIcon(): React.ReactElement {
  return (
    <svg width="18" height="18" viewBox="-3 -3 30 30" fill="currentColor" aria-hidden="true">
      <path d="M22.282 9.821a5.985 5.985 0 0 0-.516-4.911 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.985 5.985 0 0 0-3.998 2.9 6.046 6.046 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.514 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.99 5.99 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073zm-8.33 11.69a4.476 4.476 0 0 1-2.876-1.04l.141-.081 4.779-2.758a.796.796 0 0 0 .392-.68v-6.738l2.02 1.168a.07.07 0 0 1 .038.053v5.582a4.504 4.504 0 0 1-4.494 4.494zm-9.652-3.82a4.47 4.47 0 0 1-.535-3.014l.141.085 4.784 2.759a.77.77 0 0 0 .78 0l5.843-3.369v2.333a.08.08 0 0 1-.033.062L9.74 19.95a4.499 4.499 0 0 1-6.14-1.647zM2.34 7.896a4.485 4.485 0 0 1 2.366-1.973V11.6a.767.767 0 0 0 .388.677l5.815 3.354-2.02 1.168a.076.076 0 0 1-.071 0l-4.83-2.786A4.504 4.504 0 0 1 2.34 7.872zm16.597 3.855l-5.833-3.387L15.118 7.2a.076.076 0 0 1 .071 0l4.83 2.791a4.494 4.494 0 0 1-.677 8.105v-5.678a.79.79 0 0 0-.407-.667zm2.01-3.023l-.141-.085-4.774-2.782a.776.776 0 0 0-.784 0L9.41 9.23V6.898a.066.066 0 0 1 .028-.061l4.83-2.787a4.5 4.5 0 0 1 6.679 4.66zm-12.64 4.134l-2.02-1.164a.08.08 0 0 1-.038-.057V6.074a4.5 4.5 0 0 1 7.374-3.453l-.142.08-4.777 2.758a.795.795 0 0 0-.393.681zm1.097-2.365l2.602-1.5 2.607 1.5v2.999l-2.597 1.5-2.607-1.5Z" />
    </svg>
  );
}

function ClaudeIcon(): React.ReactElement {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <rect x="10.75" y="5.5" width="2.5" height="13" rx="1.25" />
      <rect x="10.75" y="5.5" width="2.5" height="13" rx="1.25" transform="rotate(45 12 12)" />
      <rect x="10.75" y="5.5" width="2.5" height="13" rx="1.25" transform="rotate(90 12 12)" />
      <rect x="10.75" y="5.5" width="2.5" height="13" rx="1.25" transform="rotate(135 12 12)" />
    </svg>
  );
}

function GeminiIcon(): React.ReactElement {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 2c0 5.52-4.48 10-10 10 5.52 0 10 4.48 10 10 0-5.52 4.48-10 10-10-5.52 0-10-4.48-10-10z" />
    </svg>
  );
}

function StopIcon(): React.ReactElement {
  return (
    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" aria-hidden="true">
      <rect x="2" y="2" width="10" height="10" rx="2" />
    </svg>
  );
}

function SwitchAccountIcon(): React.ReactElement {
  return (
    <svg width="13" height="13" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <polyline points="2,4 12,4 9,1" />
      <polyline points="12,10 2,10 5,13" />
    </svg>
  );
}

const PROVIDER_CARDS: { value: ProviderKey; label: string; icon: React.ReactElement }[] = [
  { value: "codex", label: "Codex", icon: <CodexIcon /> },
  { value: "claude", label: "Claude", icon: <ClaudeIcon /> },
  { value: "gemini", label: "Gemini", icon: <GeminiIcon /> },
];

// Providers whose runner adapters advertise the Vision capability (Task-052). Mirrors
// ProviderCapabilities.Vision in the Go runner; a follow-up should source this from the
// provider registration the renderer loads instead of hardcoding it here.
const VISION_PROVIDERS = new Set<ProviderKey>(["codex", "claude"]);

const REASONING_OPTIONS = [
  { value: "", label: "Default" },
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
];

function skillSourceLabel(source: "provider" | "flowpilot" | "workspace"): string {
  return source === "provider" ? "Account" : "Project";
}

function formatTokenCount(value: number): string {
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(value >= 10_000_000 ? 0 : 1)}M`;
  }
  if (value >= 1_000) {
    return `${(value / 1_000).toFixed(value >= 10_000 ? 0 : 1)}k`;
  }
  return String(value);
}

function usageNumber(value: number): React.ReactElement {
  return <span className="chat-usage-number">{formatTokenCount(value)}</span>;
}

function usageSeparator(): React.ReactElement {
  return <span className="chat-usage-separator"> · </span>;
}

function usageSummaryLine(provider: ProviderKey | undefined, usage: TokenUsageSnapshot | undefined): ReactNode | null {
  if (!provider) return null;
  if (!usage) return null;

  const last = usage.last;
  const total = usage.total;
  const windowSize = usage.modelContextWindow ?? null;
  const contextUsed = total?.totalTokens ?? last?.totalTokens ?? null;
  const parts: ReactNode[] = [];

  if (windowSize && contextUsed !== null) {
    const remaining = Math.max(windowSize - contextUsed, 0);
    parts.push(
      <span key="context">
        Context {usageNumber(contextUsed)} / {usageNumber(windowSize)} used
      </span>,
    );
    parts.push(
      <span key="remaining">
        {usageNumber(remaining)} left
      </span>,
    );
  }

  if (last) {
    parts.push(
      <span key="last-turn">
        Last turn {usageNumber(last.totalTokens)} tokens
      </span>,
    );
    parts.push(
      <span key="in-out">
        in {usageNumber(last.inputTokens)} · out {usageNumber(last.outputTokens)}
      </span>,
    );
  }

  if (parts.length === 0 && total) {
    parts.push(
      <span key="total">
        Total {usageNumber(total.totalTokens)} tokens
      </span>,
    );
  }

  if (parts.length === 0) return null;
  return parts.map((part, index) => (
    <span key={index}>
      {index > 0 ? usageSeparator() : null}
      {part}
    </span>
  ));
}

type SkillToken = { name: string; start: number; end: number };

// Build the backdrop children: plain strings interleaved with highlighted <mark> spans.
function buildBackdrop(text: string, tokens: SkillToken[]): React.ReactNode[] {
  const sorted = [...tokens].sort((a, b) => a.start - b.start);
  const parts: React.ReactNode[] = [];
  let pos = 0;
  for (const token of sorted) {
    if (token.start > pos) parts.push(text.slice(pos, token.start));
    parts.push(
      <mark key={`${token.name}-${token.start}`} className="skill-token-highlight">
        {text.slice(token.start, token.end)}
      </mark>,
    );
    pos = token.end;
  }
  if (pos < text.length) parts.push(text.slice(pos));
  return parts;
}

// Scan backwards from `cursor` to find an active slash command fragment.
// Returns the index of '/' and the query text, or null if none found.
// Valid triggers: '/' at position 0, or preceded by a space.
function findActiveSlash(text: string, cursor: number): { index: number; query: string } | null {
  for (let i = cursor - 1; i >= 0; i--) {
    if (text[i] === "/") {
      if (i === 0 || text[i - 1] === " ") {
        return { index: i, query: text.slice(i + 1, cursor).toLowerCase() };
      }
      return null;
    }
    if (text[i] === " ") return null;
  }
  return null;
}

export type MentionRoutingDecision =
  | { kind: "focus"; agentName: string; runId: string; prompt: string }
  | { kind: "busy"; agentName: string }
  | { kind: "missing"; agentName: string };

export function parseMentionRouting(
  text: string,
  agentRuns: Array<{ agentName: string; runId: string; status: string }>,
): MentionRoutingDecision | null {
  const trimmed = text.trim();
  const match = trimmed.match(/^@([A-Za-z0-9_-]+)\s*(.*)$/s);
  if (!match) return null;
  const agentName = match[1];
  const prompt = match[2].trim();
  const target = agentRuns.find((run) => run.agentName.toLowerCase() === agentName.toLowerCase());
  if (!target) return { kind: "missing", agentName };
  if (target.status === "running" || target.status === "waiting_approval" || target.status === "waiting_question") {
    return { kind: "busy", agentName };
  }
  return { kind: "focus", agentName, runId: target.runId, prompt };
}

// Chat composer + bottom controller strip. The controller keeps the current
// skill picker active while visually de-emphasizing the other workspace
// controls so the desktop layout reads like the mockup.
export function ChatInput(): React.ReactElement {
  const skills = useStore((s) => s.skills);
  const projects = useStore((s) => s.projects);
  const sendPrompt = useStore((s) => s.sendPrompt);
  const status = useStore((s) => s.status);
  const runId = useStore((s) => s.runId);
  const chatMode = useStore((s) => s.chatMode);
  const launchMode = useStore((s) => s.launchMode);
  const supportedModels = useStore((s) => s.supportedModels);
  const selectedProjectId = useStore((s) => s.selectedProjectId);
  const selectedWorkflowId = useStore((s) => s.selectedWorkflowId);
  const selectedStepId = useStore((s) => s.selectedStepId);
  const selectedProvider = useStore((s) => s.selectedProvider);
  const selectedModel = useStore((s) => s.selectedModel);
  const reasoningEffort = useStore((s) => s.reasoningEffort);
  const yoloMode = useStore((s) => s.yoloMode);
  const loadSkills = useStore((s) => s.loadSkills);
  const selectProvider = useStore((s) => s.selectProvider);
  const setSelectedModel = useStore((s) => s.setSelectedModel);
  const setReasoningEffort = useStore((s) => s.setReasoningEffort);
  const setYoloMode = useStore((s) => s.setYoloMode);
  const pendingApproval = useStore((s) => s.pendingApproval);
  const pendingQuestion = useStore((s) => s.pendingQuestion);
  const latestTokenUsage = useStore((s) => s.latestTokenUsage);
  const stop = useStore((s) => s.stop);
  const timeline = useStore((s) => s.timeline);
  const providerAccounts = useStore((s) => s.providerAccounts);
  const agentRuns = useStore((s) => s.agentRuns);
  const focusAgentRun = useStore((s) => s.focusAgentRun);
  const appendSystemMessage = useStore((s) => s.appendSystemMessage);
  const openAgentSpawnGuide = useStore((s) => s.openAgentSpawnGuide);
  const pendingAccountSwitch = useStore((s) => s.pendingAccountSwitch);
  const accountSwitchLoading = useStore((s) => s.accountSwitchLoading);
  const requestManualAccountSwitch = useStore((s) => s.requestManualAccountSwitch);

  const [text, setText] = useState("");
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]);
  const [pickerSortSelection, setPickerSortSelection] = useState<string[]>([]);
  const [pickerSearch, setPickerSearch] = useState("");
  const [skillPickerOpen, setSkillPickerOpen] = useState(false);
  const [cursorPos, setCursorPos] = useState(0);
  const [slashDismissedIndex, setSlashDismissedIndex] = useState<number | null>(null);
  const [skillTokens, setSkillTokens] = useState<SkillToken[]>([]);
  const [pickerHighlightIndex, setPickerHighlightIndex] = useState(-1);
  const [controllerExpanded, setControllerExpanded] = useState(true);
  const [attachments, setAttachments] = useState<PendingAttachment[]>([]);
  const [attachError, setAttachError] = useState<string | null>(null);
  const [previewAtt, setPreviewAtt] = useState<PendingAttachment | null>(null);
  const [displayedTokenUsage, setDisplayedTokenUsage] = useState<TokenUsageSnapshot | undefined>(undefined);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const textAreaRef = useRef<HTMLTextAreaElement>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const wasPickerVisibleRef = useRef(false);

  const isChatMode = chatMode === "normal_chat";
  // Lock provider once any turn has been sent in the current chat session.
  const providerLocked = isChatMode && timeline.length > 0;
  const supportsVision = !!selectedProvider && VISION_PROVIDERS.has(selectedProvider);
  const hasSelectedProject = !!selectedProjectId;
  const selectedProjectPath = useMemo(
    () => projects.find((project) => project.id === selectedProjectId)?.path,
    [projects, selectedProjectId],
  );
  const availableModels = useMemo(
    () =>
      supportedModels
        .filter((model) => model.providerKey === selectedProvider && model.isEnabled)
        .sort((a, b) => a.sortOrder - b.sortOrder),
    [selectedProvider, supportedModels],
  );
  const slashFragment = useMemo(() => {
    if (!isChatMode) return null;
    const frag = findActiveSlash(text, cursorPos);
    if (frag !== null && frag.index === slashDismissedIndex) return null;
    return frag;
  }, [isChatMode, text, cursorPos, slashDismissedIndex]);
  const slashQuery = slashFragment?.query ?? null;
  const showPicker = isChatMode && !!selectedProvider && (skillPickerOpen || slashFragment !== null);
  const totalSkills = skills.length;
  const filtered = useMemo(
    () => {
      if (!showPicker) return [];
      const query = pickerSearch.trim().toLowerCase();
      const matchingSkills = query.length === 0
        ? skills
        : skills.filter((s) => s.name.toLowerCase().includes(query));
      const sortedSkills = [...matchingSkills].sort((left, right) => left.name.localeCompare(right.name));
      const selected = sortedSkills.filter((skill) => pickerSortSelection.includes(skill.name));
      const remaining = sortedSkills.filter((skill) => !pickerSortSelection.includes(skill.name));
      return [...selected, ...remaining];
    },
    [showPicker, pickerSearch, skills, pickerSortSelection],
  );

  useEffect(() => {
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

  useEffect(() => {
    if (!isChatMode) return;
    if (availableModels.length === 0) return;
    if (!selectedModel) return;
    if (!availableModels.some((model) => model.modelId === selectedModel)) {
      setSelectedModel(undefined);
    }
  }, [availableModels, isChatMode, selectedModel, setSelectedModel]);

  useEffect(() => {
    if (isChatMode) {
      setControllerExpanded(true);
    }
  }, [isChatMode, runId]);

  // Drop pending attachments when leaving chat mode or when the selected provider
  // cannot accept images (Task-052) — they would have nowhere valid to go.
  useEffect(() => {
    if (!isChatMode || !supportsVision) {
      setAttachments([]);
      setAttachError(null);
    }
  }, [isChatMode, supportsVision]);

  useEffect(() => {
    if (!previewAtt) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setPreviewAtt(null); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [previewAtt]);

  useEffect(() => {
    setSelectedSkills((prev) => prev.filter((name) => skills.some((skill) => skill.name === name)));
  }, [skills]);

  useEffect(() => {
    if (showPicker && !wasPickerVisibleRef.current) {
      setPickerSortSelection(selectedSkills);
      searchInputRef.current?.focus();
    }
    wasPickerVisibleRef.current = showPicker;
  }, [showPicker, selectedSkills]);

  // Keep pickerSearch in sync with the slash query so typing /foo in the textarea
  // still drives the in-picker filter in real time.
  useEffect(() => {
    if (slashQuery !== null) setPickerSearch(slashQuery);
  }, [slashQuery]);

  // Clear the search box whenever the picker is dismissed.
  useEffect(() => {
    if (!showPicker) setPickerSearch("");
  }, [showPicker]);

  // Reset keyboard highlight whenever the filtered list changes.
  useEffect(() => {
    setPickerHighlightIndex(-1);
  }, [pickerSearch]);

  useEffect(() => {
    if (!showPicker) return;
    const onPointerDown = (event: PointerEvent) => {
      const root = rootRef.current;
      if (!root || root.contains(event.target as Node)) return;
      setSkillPickerOpen(false);
      setCursorPos(0);
    };
    window.addEventListener("pointerdown", onPointerDown);
    return () => window.removeEventListener("pointerdown", onPointerDown);
  }, [showPicker]);

  useEffect(() => {
    if (latestTokenUsage) {
      setDisplayedTokenUsage(latestTokenUsage);
    }
  }, [latestTokenUsage]);

  useEffect(() => {
    setDisplayedTokenUsage(undefined);
  }, [selectedProvider]);

  const hasBetterAccount = useMemo(
    () =>
      !!selectedProvider &&
      providerAccounts.some((a) => a.providerKey === selectedProvider && a.authStatus === "connected" && !a.isActive),
    [providerAccounts, selectedProvider],
  );

  const blocked = status === "running" || status === "waiting_approval" || status === "waiting_question";
  const usageLine = useMemo(
    () => usageSummaryLine(selectedProvider, displayedTokenUsage),
    [displayedTokenUsage, selectedProvider],
  );

  const canSend = isChatMode
    ? hasSelectedProject && !!selectedProvider && !blocked && text.trim().length > 0 && !showPicker
    : hasSelectedProject &&
      (launchMode === "workflow" ? !!selectedWorkflowId : !!selectedStepId) &&
      !blocked &&
      text.trim().length > 0;

  // Keep the skill picker multi-select active; the controller strip visually
  // downplays the other options but still reflects the current runtime state.
  const pickSkill = (name: string) => {
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

  const removeSkill = (name: string) => {
    setSelectedSkills((prev) => prev.filter((s) => s !== name));
  };

  const onPaste = (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    if (!supportsVision || !isChatMode || blocked) return;
    const items = Array.from(e.clipboardData?.items ?? []).filter(
      (item) => item.kind === "file" && item.type.startsWith("image/"),
    );
    if (items.length === 0) return;
    e.preventDefault();
    const dt = new DataTransfer();
    for (const item of items) {
      const file = item.getAsFile();
      if (file) dt.items.add(file);
    }
    if (dt.files.length > 0) void onPickFiles(dt.files);
  };

  const onPickFiles = async (fileList: FileList | null) => {
    if (!fileList || fileList.length === 0) return;
    setAttachError(null);
    const files = Array.from(fileList);
    const room = MAX_ATTACHMENTS - attachments.length;
    if (room <= 0) {
      setAttachError(`Up to ${MAX_ATTACHMENTS} images per message.`);
      return;
    }
    const accepted = files.slice(0, room);
    const next: PendingAttachment[] = [];
    for (const file of accepted) {
      try {
        next.push(await normalizeImage(file));
      } catch (err) {
        setAttachError(
          err instanceof ImageNormalizeError ? err.message : `Could not attach ${file.name}.`,
        );
      }
    }
    if (next.length > 0) setAttachments((prev) => [...prev, ...next]);
    if (files.length > room) setAttachError(`Up to ${MAX_ATTACHMENTS} images per message.`);
  };

  const removeAttachment = (id: string) => {
    setAttachments((prev) => prev.filter((a) => a.id !== id));
  };

  const send = async () => {
    if (!canSend) return;
    const trimmed = text.trim();
    const routed = parseMentionRouting(trimmed, agentRuns);
    if (routed) {
      if (routed.kind === "missing") {
        appendSystemMessage(`No child run named @${routed.agentName}. Open Agents and spawn it first.`);
        openAgentSpawnGuide(routed.agentName);
        return;
      }
      if (routed.kind === "busy") {
        appendSystemMessage(`@${routed.agentName} is busy right now. It cannot be interrupted or queued.`);
        return;
      }
      await focusAgentRun(routed.runId);
      if (routed.prompt.length === 0) {
        appendSystemMessage(`Focused @${routed.agentName}. Add a prompt to send work to this child run.`);
        return;
      }
      await sendPrompt(routed.prompt, isChatMode ? selectedSkills : undefined, isChatMode && attachments.length > 0 ? attachments.map(toWire) : undefined);
      setText("");
      setSelectedSkills([]);
      setSkillTokens([]);
      setAttachments([]);
      setAttachError(null);
      setSkillPickerOpen(false);
      return;
    }
    const wireAttachments =
      isChatMode && attachments.length > 0 ? attachments.map(toWire) : undefined;
    await sendPrompt(trimmed, isChatMode ? selectedSkills : undefined, wireAttachments);
    setText("");
    setSelectedSkills([]);
    setSkillTokens([]);
    setAttachments([]);
    setAttachError(null);
    setSkillPickerOpen(false);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
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
          if (selectedSkills.includes(highlighted.name)) removeSkill(highlighted.name);
          else pickSkill(highlighted.name);
        }
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        if (slashFragment !== null) setSlashDismissedIndex(slashFragment.index);
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

  const placeholder = blocked
    ? "Waiting for the current turn..."
    : !hasSelectedProject
      ? "Select a project first."
    : isChatMode
      ? selectedProvider
        ? "Type a message. Use / anywhere to pick a skill."
        : "Select a provider first."
      : launchMode === "workflow"
        ? selectedWorkflowId
          ? "Type a message. Enter to send."
          : "Select a workflow first."
        : selectedStepId
          ? "Type a message. Enter to send."
          : "Select a step first.";

  return (
    <div ref={rootRef} className={`chat-input ${!hasSelectedProject ? "chat-input-locked" : ""}`}>
      {isChatMode && (
        <div className={`chat-controller ${controllerExpanded ? "expanded" : "collapsed"}`}>
          {controllerExpanded && (
            <>
              <div className="chat-controller-top">
                <div className="chat-controller-skill-strip">
                  <span className="chat-controller-label">Skills</span>
                  <button
                    type="button"
                    className={`skill-select-button ${showPicker ? "active" : ""}`}
                    onClick={() => setSkillPickerOpen((current) => !current)}
                    aria-expanded={showPicker}
                    disabled={!selectedProvider || blocked}
                    aria-label={
                      selectedProvider
                        ? `Selected skills ${selectedSkills.length} of ${totalSkills}`
                        : "Select skills"
                    }
                  >
                    <span className="skill-select-icon" aria-hidden="true">
                      <span />
                      <span />
                      <span />
                    </span>
                    <span className="skill-select-text">
                      {!selectedProvider
                        ? "Select provider first"
                        : `Selected skills ${selectedSkills.length}/${totalSkills}`}
                    </span>
                    <span className="skill-select-caret" aria-hidden="true">
                      ▾
                    </span>
                  </button>
                </div>
                <div className="chat-controller-head">
                  {isChatMode && hasBetterAccount && (
                    <button
                      type="button"
                      className="acc-switch-btn"
                      onClick={requestManualAccountSwitch}
                      disabled={blocked || !!pendingAccountSwitch || accountSwitchLoading}
                      title="Switch to a better account for this provider"
                      aria-label="Switch to a better account"
                    >
                      <SwitchAccountIcon />
                      <span>Switch acc</span>
                    </button>
                  )}
                  <div className="chat-controller-switch chat-controller-switch-top">
                    <span className="chat-controller-label">YOLO</span>
                    <button
                      type="button"
                      role="switch"
                      aria-checked={yoloMode}
                      className={`yolo-toggle ${yoloMode ? "active" : ""}`}
                      onClick={() => setYoloMode(!yoloMode)}
                      disabled={blocked}
                    >
                      <span className="yolo-toggle-track" aria-hidden="true">
                        <span className="yolo-toggle-thumb" />
                      </span>
                      <span className="yolo-toggle-label">{yoloMode ? "On" : "Off"}</span>
                    </button>
                  </div>
                  <button
                    type="button"
                    className="chat-controller-toggle"
                    onClick={() => setControllerExpanded(false)}
                    aria-label="Collapse chat controls"
                  >
                    <span aria-hidden="true">▾</span>
                  </button>
                </div>
              </div>

              <div className="chat-controller-grid">
                <div className={`provider-picker${providerLocked ? " provider-picker-locked" : ""}`}>
                  <span className="chat-controller-label">Provider</span>
                  <div className="provider-chips">
                    {PROVIDER_CARDS.map((p) => (
                      <button
                        key={p.value}
                        type="button"
                        className={`provider-chip provider-chip-${p.value}${selectedProvider === p.value ? " provider-chip-selected" : ""}`}
                        onClick={() => selectProvider(selectedProvider === p.value ? undefined : p.value)}
                        disabled={blocked || providerLocked}
                        aria-pressed={selectedProvider === p.value}
                        aria-label={p.label}
                        title={providerLocked ? "Start a new chat to change provider" : p.label}
                      >
                        <span className="provider-chip-icon">{p.icon}</span>
                        <span className="provider-chip-name">{p.label}</span>
                      </button>
                    ))}
                  </div>
                </div>

                <label className="chat-controller-field muted">
                  <span>Model</span>
                  {availableModels.length > 0 ? (
                    <select
                      value={selectedModel ?? ""}
                      onChange={(e) => setSelectedModel(e.target.value || undefined)}
                      disabled={blocked}
                    >
                      <option value="">Default</option>
                      {availableModels.map((model) => (
                        <option key={model.id} value={model.modelId}>
                          {model.displayName}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <input
                      type="text"
                      value={selectedModel ?? ""}
                      onChange={(e) => setSelectedModel(e.target.value || undefined)}
                      placeholder="Default"
                      disabled={blocked}
                    />
                  )}
                </label>

                <label className="chat-controller-field muted">
                  <span>Reasoning</span>
                  <select
                    value={reasoningEffort ?? ""}
                    onChange={(e) => setReasoningEffort(e.target.value || undefined)}
                    disabled={blocked}
                  >
                    {REASONING_OPTIONS.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
            </>
          )}
        </div>
      )}

      {showPicker && (
        <div className="skill-picker">
          <div className="skill-picker-head">
            <div className="skill-picker-head-top">
              <span>Skills · pick one or more</span>
              <button
                type="button"
                className="skill-picker-close"
                onClick={() => {
                  if (slashFragment !== null) setSlashDismissedIndex(slashFragment.index);
                  setSkillPickerOpen(false);
                  setPickerHighlightIndex(-1);
                }}
                aria-label="Close skills picker"
              >
                ×
              </button>
            </div>
            <input
              ref={searchInputRef}
              type="text"
              className="skill-picker-search"
              placeholder="Search skills…"
              value={pickerSearch}
              onChange={(e) => setPickerSearch(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  setPickerHighlightIndex((prev) => Math.min(prev + 1, filtered.length - 1));
                } else if (e.key === "ArrowUp") {
                  e.preventDefault();
                  setPickerHighlightIndex((prev) => Math.max(prev - 1, 0));
                } else if (e.key === "Enter" && pickerHighlightIndex >= 0) {
                  e.preventDefault();
                  const highlighted = filtered[pickerHighlightIndex];
                  if (highlighted) {
                    if (selectedSkills.includes(highlighted.name)) removeSkill(highlighted.name);
                    else pickSkill(highlighted.name);
                  }
                } else if (e.key === "Escape") {
                  e.preventDefault();
                  if (slashFragment !== null) setSlashDismissedIndex(slashFragment.index);
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
              }}
              aria-label="Search skills"
            />
          </div>
          {filtered.length === 0 && <div className="skill-empty">No matching skill</div>}
          {filtered.map((s, idx) => {
            const active = selectedSkills.includes(s.name);
            const highlighted = idx === pickerHighlightIndex;
            return (
              <button
                key={s.name}
                type="button"
                className={`skill-item ${active ? "skill-item-active" : ""} ${highlighted ? "skill-item-highlighted" : ""}`}
                onClick={() => (active ? removeSkill(s.name) : pickSkill(s.name))}
              >
                <span className="skill-mark">{active ? "☑" : "☐"}</span>
                <span className="skill-copy">
                  <span className={`skill-name ${active ? "skill-name-active" : "skill-name-idle"}`}>/{s.name}</span>
                  {s.description && <span className="skill-desc" title={s.description}>{s.description}</span>}
                </span>
                <span className={`skill-src src-${s.source}`}>{skillSourceLabel(s.source)}</span>
              </button>
            );
          })}
        </div>
      )}

      {isChatMode && supportsVision && attachments.length > 0 && (
        <div className="chat-attachments" aria-label="Pending image attachments">
          {attachments.map((att) => (
            <div
              key={att.id}
              className="chat-attachment-chip"
              role="button"
              tabIndex={0}
              onClick={() => setPreviewAtt(att)}
              onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") setPreviewAtt(att); }}
              aria-label={`Preview ${att.originalName}`}
              title="Click to preview"
            >
              <img className="chat-attachment-thumb" src={att.previewUrl} alt={att.originalName} />
              <span className="chat-attachment-name" title={att.originalName}>
                {att.originalName}
              </span>
              <button
                type="button"
                className="chat-attachment-remove"
                onClick={(e) => { e.stopPropagation(); removeAttachment(att.id); }}
                disabled={blocked}
                aria-label={`Remove ${att.originalName}`}
              >
                ×
              </button>
            </div>
          ))}
        </div>
      )}
      {isChatMode && attachError && (
        <div className="chat-attachment-error" role="alert">
          {attachError}
        </div>
      )}

      <div className="input-bar">
        {isChatMode && !controllerExpanded && (
          <button
            type="button"
            className="chat-controller-toggle chat-controller-toggle-inline"
            onClick={() => setControllerExpanded(true)}
            aria-label="Expand chat controls"
          >
            <span className="chat-controller-menu-icon" aria-hidden="true">
              <span />
              <span />
              <span />
            </span>
          </button>
        )}
        {isChatMode && (
          <>
            <input
              ref={fileInputRef}
              type="file"
              accept={ACCEPT_ATTR}
              multiple
              hidden
              onChange={(e) => {
                void onPickFiles(e.target.files);
                e.target.value = ""; // allow re-picking the same file
              }}
            />
            <button
              type="button"
              className="attach-btn"
              onClick={() => fileInputRef.current?.click()}
              disabled={!supportsVision || blocked || attachments.length >= MAX_ATTACHMENTS}
              aria-label={
                supportsVision
                  ? "Attach image"
                  : "Image attachments are not supported by the selected provider"
              }
              title={supportsVision ? "Attach image (or paste with Ctrl+V)" : "Selected provider does not support images"}
            >
              <span aria-hidden="true">📎</span>
              {attachments.length > 0 && (
                <span className="attach-badge" aria-label={`${attachments.length} image${attachments.length > 1 ? "s" : ""} attached`}>
                  {attachments.length}
                </span>
              )}
            </button>
          </>
        )}
        <div className={`text-area-wrapper${isChatMode && skillTokens.length > 0 ? " has-highlights" : ""}`}>
          {isChatMode && skillTokens.length > 0 && (
            <div className="text-area-backdrop" aria-hidden="true">
              {buildBackdrop(text, skillTokens)}
            </div>
          )}
          <textarea
            ref={textAreaRef}
            className="text-area"
            rows={2}
            placeholder={placeholder}
            value={text}
            onChange={(e) => {
              const newText = e.target.value;
              const newCursor = e.target.selectionStart ?? 0;
              setText(newText);
              setCursorPos(newCursor);
              if (slashDismissedIndex !== null && newText[slashDismissedIndex] !== "/") {
                setSlashDismissedIndex(null);
              }
              setSkillTokens((prev) =>
                prev.filter((t) => newText.slice(t.start, t.end) === t.name),
              );
            }}
            onKeyDown={onKeyDown}
            onPaste={onPaste}
            onSelect={(e) => setCursorPos((e.target as HTMLTextAreaElement).selectionStart ?? 0)}
            onPointerDown={() => {
              if (skillPickerOpen) setSkillPickerOpen(false);
            }}
          />
        </div>
        {blocked ? (
          <button className="btn send-btn send-btn-stop" onClick={() => void stop()} aria-label="Stop AI">
            <StopIcon />
          </button>
        ) : (
          <button className="btn btn-primary send-btn" onClick={send} disabled={!canSend}>
            Send
          </button>
        )}
      </div>

      {isChatMode && usageLine && (
        <div className="chat-usage-line" aria-live="polite">
          {usageLine}
        </div>
      )}

      {(pendingApproval || pendingQuestion) && (
        <div className="input-note">Action required above before continuing.</div>
      )}

      {previewAtt && (
        <div
          className="attach-preview-overlay"
          role="dialog"
          aria-modal="true"
          aria-label={`Preview: ${previewAtt.originalName}`}
          onClick={() => setPreviewAtt(null)}
        >
          <div className="attach-preview-box" onClick={(e) => e.stopPropagation()}>
            <div className="attach-preview-header">
              <span className="attach-preview-name">{previewAtt.originalName}</span>
              <span className="attach-preview-meta">
                {previewAtt.width && previewAtt.height ? `${previewAtt.width}×${previewAtt.height} · ` : ""}
                {(previewAtt.sizeBytes / 1024).toFixed(0)} KB
              </span>
              <button
                type="button"
                className="attach-preview-close"
                onClick={() => setPreviewAtt(null)}
                aria-label="Close preview"
              >
                ×
              </button>
            </div>
            <img
              className="attach-preview-img"
              src={previewAtt.previewUrl}
              alt={previewAtt.originalName}
            />
          </div>
        </div>
      )}

      {!hasSelectedProject && (
        <div className={`chat-input-guard ${isChatMode ? "" : "chat-input-guard-compact"}`.trim()} aria-live="polite">
          <div className={`chat-input-guard-card ${isChatMode ? "" : "chat-input-guard-card-compact"}`.trim()}>
            <div className="chat-input-guard-title">Select a project first</div>
            <div className="chat-input-guard-copy">
              {isChatMode
                ? "Choose a project before using chat, skills, or send."
                : "Choose a project before continuing in workflow mode."}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
