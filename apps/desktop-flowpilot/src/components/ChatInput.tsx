import { useEffect, useMemo, useRef, useState } from "react";
import { useStore } from "@/state/store";
import type { ProviderKey } from "@/types/contract";
import {
  ACCEPT_ATTR,
  MAX_ATTACHMENTS,
  ImageNormalizeError,
  normalizeImage,
  toWire,
  type PendingAttachment,
} from "@/lib/normalizeImage";

const PROVIDER_OPTIONS: { value: ProviderKey; label: string }[] = [
  { value: "codex", label: "Codex" },
  { value: "claude", label: "Claude" },
  { value: "gemini", label: "Gemini" },
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

  const [text, setText] = useState("");
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]);
  const [pickerSortSelection, setPickerSortSelection] = useState<string[]>([]);
  const [skillPickerOpen, setSkillPickerOpen] = useState(false);
  const [controllerExpanded, setControllerExpanded] = useState(true);
  const [attachments, setAttachments] = useState<PendingAttachment[]>([]);
  const [attachError, setAttachError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const wasPickerVisibleRef = useRef(false);

  const isChatMode = chatMode === "normal_chat";
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
  const slashQuery = isChatMode && text.startsWith("/") ? text.slice(1).toLowerCase() : null;
  const showPicker = isChatMode && !!selectedProvider && (skillPickerOpen || slashQuery !== null);
  const totalSkills = skills.length;
  const filtered = useMemo(
    () => {
      if (!showPicker) return [];
      const query = slashQuery?.trim() ?? "";
      const matchingSkills = query.length === 0
        ? skills
        : skills.filter((s) => s.name.toLowerCase().includes(query));
      const sortedSkills = [...matchingSkills].sort((left, right) => left.name.localeCompare(right.name));
      const selected = sortedSkills.filter((skill) => pickerSortSelection.includes(skill.name));
      const remaining = sortedSkills.filter((skill) => !pickerSortSelection.includes(skill.name));
      return [...selected, ...remaining];
    },
    [showPicker, slashQuery, skills, pickerSortSelection],
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
    setSelectedSkills((prev) => prev.filter((name) => skills.some((skill) => skill.name === name)));
  }, [skills]);

  useEffect(() => {
    if (showPicker && !wasPickerVisibleRef.current) {
      setPickerSortSelection(selectedSkills);
    }
    wasPickerVisibleRef.current = showPicker;
  }, [showPicker, selectedSkills]);

  useEffect(() => {
    if (!showPicker) return;
    const onPointerDown = (event: PointerEvent) => {
      const root = rootRef.current;
      if (!root || root.contains(event.target as Node)) return;
      setSkillPickerOpen(false);
      if (slashQuery !== null) {
        setText("");
      }
    };
    window.addEventListener("pointerdown", onPointerDown);
    return () => window.removeEventListener("pointerdown", onPointerDown);
  }, [showPicker, slashQuery]);

  const blocked = status === "running" || status === "waiting_approval" || status === "waiting_question";

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
    if (slashQuery !== null) {
      setText("");
      setSkillPickerOpen(false);
    }
  };

  const removeSkill = (name: string) => {
    setSelectedSkills((prev) => prev.filter((s) => s !== name));
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

  const send = () => {
    if (!canSend) return;
    const wireAttachments =
      isChatMode && attachments.length > 0 ? attachments.map(toWire) : undefined;
    void sendPrompt(text.trim(), isChatMode ? selectedSkills : undefined, wireAttachments);
    setText("");
    setSelectedSkills([]);
    setAttachments([]);
    setAttachError(null);
    setSkillPickerOpen(false);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !showPicker) {
      e.preventDefault();
      send();
    }
  };

  const placeholder = blocked
    ? "Waiting for the current turn..."
    : !hasSelectedProject
      ? "Select a project first."
    : isChatMode
      ? selectedProvider
        ? "Type a message, or / to pick a skill. Enter to send."
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
                <label className="chat-controller-field muted">
                  <span>Provider</span>
                  <select
                    value={selectedProvider ?? ""}
                    onChange={(e) =>
                      selectProvider(e.target.value ? (e.target.value as ProviderKey) : undefined)
                    }
                    disabled={blocked}
                  >
                    <option value="">Auto</option>
                    {PROVIDER_OPTIONS.map((provider) => (
                      <option key={provider.value} value={provider.value}>
                        {provider.label}
                      </option>
                    ))}
                  </select>
                </label>

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
          <div className="skill-picker-head">Skills · pick one or more</div>
          {filtered.length === 0 && <div className="skill-empty">No matching skill</div>}
          {filtered.map((s) => {
            const active = selectedSkills.includes(s.name);
            return (
              <button
                key={s.name}
                type="button"
                className={`skill-item ${active ? "skill-item-active" : ""}`}
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
            <div key={att.id} className="chat-attachment-chip">
              <img className="chat-attachment-thumb" src={att.previewUrl} alt={att.originalName} />
              <span className="chat-attachment-name" title={att.originalName}>
                {att.originalName}
              </span>
              <button
                type="button"
                className="chat-attachment-remove"
                onClick={() => removeAttachment(att.id)}
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
              title={supportsVision ? "Attach image" : "Selected provider does not support images"}
            >
              <span aria-hidden="true">📎</span>
            </button>
          </>
        )}
        <textarea
          className="text-area"
          rows={2}
          placeholder={placeholder}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
          disabled={blocked}
        />
        <button className="btn btn-primary send-btn" onClick={send} disabled={!canSend}>
          Send
        </button>
      </div>

      {(pendingApproval || pendingQuestion) && (
        <div className="input-note">Action required above before continuing.</div>
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
