import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { flushSync } from "react-dom";
import { useStore } from "@/state/store";
import { findActiveAt, insertAtMention, isAgentAtMention } from "@/components/chatFileMention";
import { findMentionSpans } from "@/components/mentionHighlight";
import type { ProviderAccountSummary, ProviderKey, TokenUsageSnapshot } from "@/types/contract";
import type { SupportedModel } from "@flowpilot/client-core";
import { contextRemainingPercent, formatAccountRemainingLabel } from "@/lib/usageSummary";
import {
  ACCEPT_ATTR,
  MAX_ATTACHMENTS,
  ImageNormalizeError,
  normalizeImage,
  toWire,
  type PendingAttachment,
} from "@/lib/normalizeImage";
import { supportsVisionFor } from "./visionProviders";

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

// Official Grok/xAI swirl mark (Wikimedia Commons Grok-icon.svg, Task-211 T-4).
function GrokIcon(): React.ReactElement {
  return (
    <svg width="18" height="18" viewBox="0 0 512 509.641" fill="currentColor" aria-hidden="true">
      <path d="M213.235 306.019l178.976-180.002v.169l51.695-51.763c-.924 1.32-1.86 2.605-2.785 3.89-39.281 54.164-58.46 80.649-43.07 146.922l-.09-.101c10.61 45.11-.744 95.137-37.398 131.836-46.216 46.306-120.167 56.611-181.063 14.928l42.462-19.675c38.863 15.278 81.392 8.57 111.947-22.03 30.566-30.6 37.432-75.159 22.065-112.252-2.92-7.025-11.67-8.795-17.792-4.263l-124.947 92.341zm-25.786 22.437l-.033.034L68.094 435.217c7.565-10.429 16.957-20.294 26.327-30.149 26.428-27.803 52.653-55.359 36.654-94.302-21.422-52.112-8.952-113.177 30.724-152.898 41.243-41.254 101.98-51.661 152.706-30.758 11.23 4.172 21.016 10.114 28.638 15.639l-42.359 19.584c-39.44-16.563-84.629-5.299-112.207 22.313-37.298 37.308-44.84 102.003-1.128 143.81z" />
    </svg>
  );
}

function OpencodeIcon(): React.ReactElement {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
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
  { value: "grok", label: "Grok", icon: <GrokIcon /> },
  { value: "opencode", label: "OpenCode", icon: <OpencodeIcon /> },
];


// Fallback reasoning-effort options (Task-215): used only when the selected
// model has no detected `supportedReasoningEfforts` in the catalog (a
// manually-added model, or a provider Task-215's detectors don't cover —
// Claude has no per-model catalog; the CLI validates `--effort` against one
// global list, live-verified by inspecting the installed
// @anthropic-ai/claude-code binary (2.1.191): `GD=["low","medium","high",
// "xhigh","max"]`, applied uniformly regardless of model). When a model DOES
// carry detected data, the Reasoning control derives its options from that
// model instead of this static list — see `reasoningOptionsFor` below.
const FALLBACK_REASONING_OPTIONS = [
  { value: "", label: "Default" },
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
  { value: "xhigh", label: "Extra High" },
  { value: "max", label: "Max" },
];

const REASONING_EFFORT_LABELS: Record<string, string> = {
  low: "Low",
  medium: "Medium",
  high: "High",
  xhigh: "Extra High",
  max: "Max",
  ultra: "Ultra",
};

function reasoningEffortLabel(effort: string): string {
  return REASONING_EFFORT_LABELS[effort] ?? effort.charAt(0).toUpperCase() + effort.slice(1);
}

// Task-215: derive the Reasoning dropdown's options from the selected
// model's own detected `supportedReasoningEfforts` (Codex/Grok already
// report this per model — see Task-215) instead of one fixed list applied
// to every model of a provider. Falls back to the static list when the
// catalog has no reasoning data for this model.
function reasoningOptionsFor(model: SupportedModel | undefined): { value: string; label: string }[] {
  const efforts = model?.supportedReasoningEfforts;
  if (!efforts || efforts.length === 0) return FALLBACK_REASONING_OPTIONS;
  return [
    { value: "", label: "Default" },
    ...efforts.map((effort) => ({ value: effort, label: reasoningEffortLabel(effort) })),
  ];
}

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

// Task-215: `usage.modelContextWindow` is reported live by the running
// provider process itself once a turn has produced usage data (Codex's
// appserver events, Grok's ACP `initialize`/`session/new`). Before a live
// value exists — e.g. no turn sent yet — `fallbackContextWindow` (the
// catalog's detected `contextWindowTokens` for the selected model) fills the
// same slot so the usage bar can show a window size from the first render.
function usageSummaryLine(
  provider: ProviderKey | undefined,
  usage: TokenUsageSnapshot | undefined,
  fallbackContextWindow?: number | null,
  account?: ProviderAccountSummary,
): ReactNode | null {
  if (!provider) return null;

  const last = usage?.last;
  const total = usage?.total;
  const windowSize = usage?.modelContextWindow ?? fallbackContextWindow ?? null;
  const contextUsed = total?.totalTokens ?? last?.totalTokens ?? null;
  const parts: ReactNode[] = [];

  const credit = formatAccountRemainingLabel(account);
  if (credit) {
    parts.push(<span key="credits">{credit}</span>);
  }

  if (windowSize && contextUsed !== null) {
    const remaining = Math.max(windowSize - contextUsed, 0);
    const remainPct = contextRemainingPercent(contextUsed, windowSize);
    parts.push(
      <span key="context">
        Context {usageNumber(contextUsed)} / {usageNumber(windowSize)} used
      </span>,
    );
    parts.push(
      <span key="remain-pct">{remainPct}% remain</span>,
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
function buildBackdrop(text: string, skillNames: string[]): React.ReactNode[] {
  const spans = findMentionSpans(text, skillNames);
  const parts: React.ReactNode[] = [];
  let pos = 0;
  for (const span of spans) {
    if (span.start > pos) parts.push(text.slice(pos, span.start));
    parts.push(
      <mark key={`${span.kind}-${span.start}`} className={`mention-token mention-${span.kind}${span.kind === "skill" ? " skill-token-highlight" : ""}`}>
        {text.slice(span.start, span.end)}
      </mark>,
    );
    pos = span.end;
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
  | { kind: "busy"; agentName: string; runId: string; prompt: string }
  | { kind: "missing"; agentName: string };

export function isChildRunFocused(activeAgentRunId: string | undefined, mainRunId: string | undefined): boolean {
  return Boolean(activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId);
}

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
    return { kind: "busy", agentName, runId: target.runId, prompt };
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
  const grokYoloPostureLoading = useStore((s) => s.grokYoloPostureLoading);
  const loadSkills = useStore((s) => s.loadSkills);
  const client = useStore((s) => s.client);
  const selectProvider = useStore((s) => s.selectProvider);
  const setSelectedModel = useStore((s) => s.setSelectedModel);
  const setReasoningEffort = useStore((s) => s.setReasoningEffort);
  const toggleYoloForActiveProvider = useStore((s) => s.toggleYoloForActiveProvider);
  const generateChatSummary = useStore((s) => s.generateChatSummary);
  const summaryGenerating = useStore((s) => s.summaryGenerating);
  const pendingApprovals = useStore((s) => s.pendingApprovals);
  const pendingQuestions = useStore((s) => s.pendingQuestions);
  const latestTokenUsage = useStore((s) => s.latestTokenUsage);
  const stop = useStore((s) => s.stop);
  const timeline = useStore((s) => s.timeline);
  const providerAccounts = useStore((s) => s.providerAccounts);
  const localProviders = useStore((s) => s.localProviders);
  const agentRuns = useStore((s) => s.agentRuns);
  const activeAgentRunId = useStore((s) => s.activeAgentRunId);
  const mainRunId = useStore((s) => s.mainRunId ?? s.runId);
  const focusAgentRun = useStore((s) => s.focusAgentRun);
  const appendSystemMessage = useStore((s) => s.appendSystemMessage);
  const openAgentSpawnGuide = useStore((s) => s.openAgentSpawnGuide);
  const pendingAccountSwitch = useStore((s) => s.pendingAccountSwitch);
  const accountSwitchLoading = useStore((s) => s.accountSwitchLoading);
  const pendingProviderSwitch = useStore((s) => s.pendingProviderSwitch);
  const providerSwitchLoading = useStore((s) => s.providerSwitchLoading);
  const requestManualAccountSwitch = useStore((s) => s.requestManualAccountSwitch);
  const backToMainRun = useStore((s) => s.backToMainRun);

  const [text, setText] = useState("");
  const [clearSeq, setClearSeq] = useState(0);
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]);
  const [pickerSortSelection, setPickerSortSelection] = useState<string[]>([]);
  const [pickerSearch, setPickerSearch] = useState("");
  const [skillPickerOpen, setSkillPickerOpen] = useState(false);
  const [cursorPos, setCursorPos] = useState(0);
  const [slashDismissedIndex, setSlashDismissedIndex] = useState<number | null>(null);
  const [atDismissedIndex, setAtDismissedIndex] = useState<number | null>(null);
  const [workspaceFiles, setWorkspaceFiles] = useState<string[]>([]);
  const [fileHighlightIndex, setFileHighlightIndex] = useState(0);
  const [skillTokens, setSkillTokens] = useState<SkillToken[]>([]);
  const [pickerHighlightIndex, setPickerHighlightIndex] = useState(-1);
  const [controllerExpanded, setControllerExpanded] = useState(true);
  const [attachments, setAttachments] = useState<PendingAttachment[]>([]);
  const [attachError, setAttachError] = useState<string | null>(null);
  const [previewAtt, setPreviewAtt] = useState<PendingAttachment | null>(null);
  const [displayedTokenUsage, setDisplayedTokenUsage] = useState<TokenUsageSnapshot | undefined>(undefined);
  // Composer height driven by the top-edge drag handle (px). null = rows={2}
  // default. Kept in component state (not tied to the textarea's clearSeq key)
  // so a dragged height survives the remount-on-send. The panel sits at the
  // bottom of the screen, so the handle lives on the TOP edge and dragging up
  // grows the box upward — a native bottom-right resize grip would grow it off
  // the bottom of the viewport where there is no room.
  const [composerHeight, setComposerHeight] = useState<number | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const textAreaRef = useRef<HTMLTextAreaElement>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const wasPickerVisibleRef = useRef(false);

  const isChatMode = chatMode === "normal_chat";
  const isSwitchBusy = pendingProviderSwitch !== undefined || providerSwitchLoading;
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
  // Task-215: the catalog row for whichever model is currently selected, if
  // any — the source of both the model-aware Reasoning options and the
  // context-window fallback for the usage bar.
  const selectedModelInfo = useMemo(
    () => availableModels.find((model) => model.modelId === selectedModel),
    [availableModels, selectedModel],
  );
  // Task-319: per-model image gate — provider-level VISION_PROVIDERS plus the
  // opencode per-model unlock (models.dev input.image on the selected model).
  const supportsVision = supportsVisionFor(selectedProvider, selectedModelInfo?.inputImage);
  const reasoningOptions = useMemo(() => reasoningOptionsFor(selectedModelInfo), [selectedModelInfo]);
  useEffect(() => {
    if (reasoningEffort === undefined) return;
    if (reasoningOptions.some((option) => option.value === reasoningEffort)) return;
    // The previously-selected effort isn't valid for the newly-selected
    // model (e.g. switching from a model that supports "xhigh" to one that
    // only supports up to "high") — degrade to the model's default rather
    // than silently sending an unsupported value to the CLI (Task-215 T-5).
    setReasoningEffort(undefined);
  }, [reasoningEffort, reasoningOptions, setReasoningEffort]);
  const slashFragment = useMemo(() => {
    if (!isChatMode) return null;
    const frag = findActiveSlash(text, cursorPos);
    if (frag !== null && frag.index === slashDismissedIndex) return null;
    return frag;
  }, [isChatMode, text, cursorPos, slashDismissedIndex]);
  const slashQuery = slashFragment?.query ?? null;
  // Slash sub-commands are namespaced by the command letter after "/": "/a…" (or "/agent")
  // opens the spawn-agent UI, "/s…" (or "/skill") opens the skill picker. A bare "/" (or any
  // other leading letter) opens nothing — a command letter is required so "/" no longer pops
  // the skill UI on its own (BUG-134). The remainder after "/s" is the skill search term.
  const slashCommand: "agent" | "skill" | null =
    slashFragment === null
      ? null
      : slashFragment.query[0] === "a"
        ? "agent"
        : slashFragment.query[0] === "s"
          ? "skill"
          : null;
  const showAgentCommand = isChatMode && !!selectedProvider && slashCommand === "agent";
  const showPicker = isChatMode && !!selectedProvider && (skillPickerOpen || slashCommand === "skill");
  const atFragment = useMemo(() => {
    if (!isChatMode) return null;
    const frag = findActiveAt(text, cursorPos);
    if (frag !== null && frag.index === atDismissedIndex) return null;
    return frag;
  }, [isChatMode, text, cursorPos, atDismissedIndex]);
  const agentNames = useMemo(() => agentRuns.map((run) => run.agentName), [agentRuns]);
  const showFilePicker = isChatMode && !!selectedProjectPath && atFragment !== null && !isAgentAtMention(atFragment, agentNames);
  const mentionSpans = useMemo(() => findMentionSpans(text, selectedSkills), [text, selectedSkills]);

  useEffect(() => {
    if (!showFilePicker || !selectedProjectPath || !client.listWorkspaceFiles) {
      setWorkspaceFiles([]);
      return;
    }
    const query = atFragment?.query ?? "";
    const handle = window.setTimeout(() => {
      void client.listWorkspaceFiles!(selectedProjectPath, query)
        .then((paths) => setWorkspaceFiles(paths))
        .catch(() => setWorkspaceFiles([]));
    }, 120);
    return () => window.clearTimeout(handle);
  }, [showFilePicker, selectedProjectPath, atFragment?.query, client]);

  useEffect(() => {
    setFileHighlightIndex(0);
  }, [atFragment?.query, workspaceFiles]);
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
    if (slashCommand !== "skill" || slashQuery === null) return;
    // The skill picker lives under the "/s" namespace, so the leading "s" (or the full
    // "skill" word) is the command, not a search term; everything after it filters the list.
    const term = slashQuery === "s" || slashQuery === "skill" ? "" : slashQuery.slice(1);
    setPickerSearch(term);
  }, [slashQuery, slashCommand]);

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

  useEffect(() => {
    setText("");
    setSelectedSkills([]);
    setSkillTokens([]);
    setAttachments([]);
    setAttachError(null);
    setPreviewAtt(null);
    setSkillPickerOpen(false);
    setSlashDismissedIndex(null);
    setAtDismissedIndex(null);
    setWorkspaceFiles([]);
    setFileHighlightIndex(0);
    setPickerHighlightIndex(-1);
  }, [runId]);

  const hasBetterAccount = useMemo(
    () =>
      !!selectedProvider &&
      providerAccounts.some((a) => a.providerKey === selectedProvider && a.authStatus === "connected" && !a.isActive),
    [providerAccounts, selectedProvider],
  );
  const installedProviders = useMemo(
    () => new Set(localProviders.filter((provider) => provider.installed).map((provider) => provider.key as ProviderKey)),
    [localProviders],
  );
  const childRunFocused = isChildRunFocused(activeAgentRunId, mainRunId);
  const focusedAgentName = childRunFocused
    ? agentRuns.find((run) => run.runId === activeAgentRunId)?.agentName ?? activeAgentRunId
    : undefined;
  const activeConnectedProviders = useMemo(
    () =>
      new Set(
        providerAccounts
          .filter((account) => account.authStatus === "connected" && account.isActive)
          .map((account) => account.providerKey),
      ),
    [providerAccounts],
  );
  const selectedProviderInstalled = !!selectedProvider && installedProviders.has(selectedProvider);
  const selectedProviderConnected = !!selectedProvider && activeConnectedProviders.has(selectedProvider);
  const readyProviders = useMemo(
    () => PROVIDER_CARDS.map((provider) => provider.value).filter((provider) => installedProviders.has(provider) && activeConnectedProviders.has(provider)),
    [activeConnectedProviders, installedProviders],
  );

  useEffect(() => {
    if (!isChatMode || runId) return;
    if (readyProviders.length === 0) return;
    if (!selectedProvider || !installedProviders.has(selectedProvider) || !activeConnectedProviders.has(selectedProvider)) {
      const fallback = readyProviders[0];
      if (fallback && fallback !== selectedProvider) {
        selectProvider(fallback);
      }
    }
  }, [activeConnectedProviders, installedProviders, isChatMode, readyProviders, runId, selectProvider, selectedProvider]);

  // Include "blocked" (flow awaiting user / escalate) so Stop stays available on
  // the main composer — previously only RunStatus / FlowAwaitingUserCard had Stop
  // while status=blocked, and dual gate UI made main Stop hard to reach (CP-51 A1).
  const blocked =
    status === "running" ||
    status === "waiting_approval" ||
    status === "waiting_question" ||
    status === "blocked";
  // The manual "Gen summary" control is available only for an existing chat that
  // is idle/completed (never mid-turn) — mirrors the runner's busy guard.
  const canGenerateSummary = isChatMode && !!runId && timeline.length > 0 && !blocked && !summaryGenerating;
  // A running child spawned with wait=true blocks the main run even when the main has no turn
  // of its own in flight (e.g. a UI wait=true spawn) — the send button must reflect that (BUG-133).
  const hasBlockingChild = agentRuns.some(
    (r) => r.waitForResult && (r.status === "running" || r.status === "waiting_approval" || r.status === "waiting_question"),
  );
  const activeAccount = useMemo(
    () => providerAccounts.find((account) => account.providerKey === selectedProvider && account.isActive),
    [providerAccounts, selectedProvider],
  );
  const usageLine = useMemo(
    () => usageSummaryLine(selectedProvider, displayedTokenUsage, selectedModelInfo?.contextWindowTokens, activeAccount),
    [activeAccount, displayedTokenUsage, selectedModelInfo, selectedProvider],
  );

  const canSend = isChatMode
    ? hasSelectedProject && selectedProviderInstalled && selectedProviderConnected && !blocked && !hasBlockingChild && !childRunFocused && text.trim().length > 0 && !showPicker && !showAgentCommand && !showFilePicker
    : hasSelectedProject &&
      (launchMode === "workflow" ? !!selectedWorkflowId : !!selectedStepId) &&
      !blocked &&
      text.trim().length > 0;

  // Keep the skill picker multi-select active; the controller strip visually
  // downplays the other options but still reflects the current runtime state.
  const pickFile = (path: string) => {
    if (atFragment === null) return;
    const next = insertAtMention(text, atFragment, cursorPos, path);
    setText(next.text);
    setCursorPos(next.cursor);
    setSkillTokens((prev) => [...prev, { name: path, start: atFragment.index, end: atFragment.index + path.length }]);
    setAtDismissedIndex(null);
    setTimeout(() => {
      if (textAreaRef.current) {
        textAreaRef.current.selectionStart = next.cursor;
        textAreaRef.current.selectionEnd = next.cursor;
        textAreaRef.current.focus();
      }
    }, 0);
  };

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

  // "/a" command: strip the slash fragment from the prompt and open the spawn-agent
  // panel (the same UI as the right sidebar), then restore the caret. (Task-087)
  const triggerAgentSlash = () => {
    if (slashFragment === null) return;
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

  // Top-edge resize handle. Dragging up grows the textarea (the panel is pinned
  // to the bottom of the screen, so it must grow upward). Move/up are bound on
  // `window` for the duration of the drag so it never stalls when the cursor
  // leaves the thin bar — the previous pointer-capture-on-the-handle approach
  // dropped events and made the drag feel dead. Clamped to [rows={2}, 60vh].
  const RESIZE_MIN = 46;
  const onResizePointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const startY = e.clientY;
    const startHeight = textAreaRef.current?.getBoundingClientRect().height ?? composerHeight ?? RESIZE_MIN;
    const max = Math.round(window.innerHeight * 0.6);
    const onMove = (ev: PointerEvent) => {
      const next = Math.min(max, Math.max(RESIZE_MIN, startHeight + (startY - ev.clientY)));
      setComposerHeight(next);
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      document.body.style.userSelect = "";
    };
    // Suppress text selection while dragging the handle.
    document.body.style.userSelect = "none";
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
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

  const clearComposer = () => {
    flushSync(() => {
      setText("");
      setClearSeq((value) => value + 1);
      setSelectedSkills([]);
      setSkillTokens([]);
      setAttachments([]);
      setAttachError(null);
      setSkillPickerOpen(false);
      setPickerSearch("");
      setPickerHighlightIndex(-1);
      setCursorPos(0);
      setSlashDismissedIndex(null);
    });
    if (textAreaRef.current) {
      textAreaRef.current.value = "";
    }
  };

  const send = async () => {
    if (!canSend) return;
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
        clearComposer();
        await useStore.getState().injectAgentFeedback(routed.runId, routed.prompt || trimmed);
        appendSystemMessage(`Queued feedback for @${routed.agentName}. It will be picked up when the child is safe to continue.`);
        return;
      }
      await focusAgentRun(routed.runId);
      if (routed.prompt.length === 0) {
        appendSystemMessage(`Focused @${routed.agentName}. Add a prompt to send work to this child run.`);
        return;
      }
      const routedAttachments = isChatMode && attachments.length > 0 ? attachments.map(toWire) : undefined;
      clearComposer();
      await sendPrompt(routed.prompt, isChatMode ? selectedSkills : undefined, routedAttachments);
      return;
    }
    const wireAttachments =
      isChatMode && attachments.length > 0 ? attachments.map(toWire) : undefined;
    clearComposer();
    await sendPrompt(trimmed, isChatMode ? selectedSkills : undefined, wireAttachments);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (showAgentCommand) {
      if (e.key === "Enter") {
        e.preventDefault();
        triggerAgentSlash();
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        if (slashFragment !== null) setSlashDismissedIndex(slashFragment.index);
        return;
      }
    }
    if (showFilePicker) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setFileHighlightIndex((prev) => Math.min(prev + 1, Math.max(workspaceFiles.length - 1, 0)));
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setFileHighlightIndex((prev) => Math.max(prev - 1, 0));
        return;
      }
      if ((e.key === "Enter" || e.key === "Tab") && workspaceFiles.length > 0) {
        e.preventDefault();
        const picked = workspaceFiles[fileHighlightIndex] ?? workspaceFiles[0];
        if (picked) pickFile(picked);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        if (atFragment !== null) setAtDismissedIndex(atFragment.index);
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
    if (e.key === "Enter" && !e.shiftKey && !showPicker && !showFilePicker) {
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
          ? !selectedProviderInstalled
            ? "Install the provider CLI first."
            : !selectedProviderConnected
              ? "Activate a connected account for this provider first."
              : "Type a message. Use /s for skills, /a to spawn an agent, @file for a path, @agent to message an agent."
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
                      onClick={() => toggleYoloForActiveProvider(!yoloMode)}
                      disabled={blocked || grokYoloPostureLoading}
                    >
                      <span className="yolo-toggle-track" aria-hidden="true">
                        <span className="yolo-toggle-thumb" />
                      </span>
                      <span className="yolo-toggle-label">
                        {grokYoloPostureLoading ? "…" : yoloMode ? "On" : "Off"}
                      </span>
                    </button>
                  </div>
                  <div className="chat-controller-switch chat-controller-switch-top">
                    <button
                      type="button"
                      className="gen-summary-btn"
                      onClick={() => void generateChatSummary()}
                      disabled={!canGenerateSummary}
                      title="Generate the rolling chat summary now (available when the chat is idle)"
                    >
                      {summaryGenerating ? "Generating…" : "Gen summary"}
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
                <div className="provider-picker">
                  <span className="chat-controller-label">Provider</span>
                  <div className="provider-chips">
                    {PROVIDER_CARDS.map((p) => (
                      <button
                        key={p.value}
                        type="button"
                        className={`provider-chip provider-chip-${p.value}${selectedProvider === p.value ? " provider-chip-selected" : ""}${installedProviders.has(p.value) && activeConnectedProviders.has(p.value) ? "" : " provider-chip-unavailable"}`}
                        onClick={() => {
                          if (selectedProvider === p.value) return;
                          selectProvider(p.value);
                        }}
                        disabled={blocked || isSwitchBusy || !installedProviders.has(p.value) || !activeConnectedProviders.has(p.value)}
                        aria-pressed={selectedProvider === p.value}
                        aria-label={p.label}
                        title={
                          !installedProviders.has(p.value)
                            ? `${p.label} is unavailable until its CLI is installed`
                            : !activeConnectedProviders.has(p.value)
                              ? `${p.label} is unavailable until a connected account is active`
                              : p.label
                        }
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
                    {reasoningOptions.map((option) => (
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

      {showFilePicker && (
        <div className="skill-picker">
          <div className="skill-picker-head">
            <div className="skill-picker-head-top">
              <span>Files · Tab or Enter to insert path</span>
              <button
                type="button"
                className="skill-picker-close"
                onClick={() => { if (atFragment !== null) setAtDismissedIndex(atFragment.index); }}
                aria-label="Close file picker"
              >
                ×
              </button>
            </div>
          </div>
          {workspaceFiles.length === 0 && <div className="skill-empty">No matching file</div>}
          {workspaceFiles.map((path, idx) => (
            <button
              key={path}
              type="button"
              className={`skill-item ${idx === fileHighlightIndex ? "skill-item-highlighted" : ""}`}
              onClick={() => pickFile(path)}
            >
              <span className="skill-copy">
                <span className="skill-name skill-name-idle">{path}</span>
              </span>
            </button>
          ))}
        </div>
      )}

      {showAgentCommand && (
        <div className="skill-picker">
          <div className="skill-picker-head">
            <div className="skill-picker-head-top">
              <span>Agent command</span>
              <button
                type="button"
                className="skill-picker-close"
                onClick={() => { if (slashFragment !== null) setSlashDismissedIndex(slashFragment.index); }}
                aria-label="Close agent command"
              >
                ×
              </button>
            </div>
          </div>
          <button
            type="button"
            className="skill-item skill-item-highlighted"
            onMouseDown={(e) => { e.preventDefault(); triggerAgentSlash(); }}
          >
            <span className="skill-mark">🤖</span>
            <span className="skill-copy">
              <span className="skill-name skill-name-idle">/a · Spawn sub-agent</span>
              <span className="skill-desc">Open the spawn-agent panel (same as the right sidebar). Press Enter.</span>
            </span>
          </button>
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
      {isChatMode && childRunFocused && (
        <div className="ctxbar ring">
          ↳ Viewing child agent <b>{focusedAgentName}</b> · transcript only
        </div>
      )}

      <div className="input-bar">
        {!childRunFocused && (
          <div
            className="composer-resize-handle"
            role="separator"
            aria-orientation="horizontal"
            aria-label="Drag to resize the message box"
            title="Drag to resize the message box (double-click to reset)"
            onPointerDown={onResizePointerDown}
            onDoubleClick={() => setComposerHeight(null)}
          >
            <span className="composer-resize-grip" aria-hidden="true" />
          </div>
        )}
        {isChatMode && !controllerExpanded && !childRunFocused && (
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
        {isChatMode && !childRunFocused && (
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
        {childRunFocused ? (
          <>
            <div className="text-area-wrapper">
              <div className="input-note">Return to the main chat to send prompts or use @agent routing.</div>
            </div>
            {blocked && (
              <button type="button" className="btn send-btn send-btn-stop" onClick={() => void stop()} aria-label="Stop child agent">
                <StopIcon />
              </button>
            )}
            <button type="button" className="btn send-btn" onClick={backToMainRun}>
              Main
            </button>
          </>
        ) : (
          <>
            <div className={`text-area-wrapper${isChatMode && mentionSpans.length > 0 ? " has-highlights" : ""}`}>
              {isChatMode && mentionSpans.length > 0 && (
                <div className="text-area-backdrop" aria-hidden="true">
                  {buildBackdrop(text, selectedSkills)}
                </div>
              )}
              <textarea
                key={clearSeq}
                ref={textAreaRef}
                className="text-area"
                rows={2}
                style={composerHeight ? { height: `${composerHeight}px` } : undefined}
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
              <button type="button" className="btn send-btn send-btn-stop" onClick={() => void stop()} aria-label="Stop AI">
                <StopIcon />
              </button>
            ) : (
              <button type="button" className="btn btn-primary send-btn" onClick={send} disabled={!canSend}>
                Send
              </button>
            )}
          </>
        )}
      </div>

      {isChatMode && usageLine && (
        <div className="chat-usage-line" aria-live="polite">
          {usageLine}
        </div>
      )}

      {(pendingApprovals.length > 0 || pendingQuestions.length > 0) && (
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
