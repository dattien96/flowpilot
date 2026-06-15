import { useEffect, useMemo, useRef, useState } from "react";
import { useStore } from "@/state/store";
import type { ProviderKey } from "@/types/contract";

const PROVIDER_OPTIONS: { value: ProviderKey; label: string }[] = [
  { value: "codex", label: "Codex" },
  { value: "claude", label: "Claude" },
  { value: "gemini", label: "Gemini" },
];

const REASONING_OPTIONS = [
  { value: "", label: "Default" },
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
];

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
  const [skillPickerOpen, setSkillPickerOpen] = useState(false);
  const [controllerExpanded, setControllerExpanded] = useState(true);
  const rootRef = useRef<HTMLDivElement>(null);

  const isChatMode = chatMode === "normal_chat";
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
  const filtered = useMemo(
    () => {
      if (!showPicker) return [];
      const query = slashQuery?.trim() ?? "";
      return query.length === 0
        ? skills
        : skills.filter((s) => s.name.toLowerCase().includes(query));
    },
    [showPicker, slashQuery, skills],
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

  useEffect(() => {
    if (!showPicker) return;
    const onPointerDown = (event: PointerEvent) => {
      const root = rootRef.current;
      if (!root || root.contains(event.target as Node)) return;
      setSkillPickerOpen(false);
      setText("");
    };
    window.addEventListener("pointerdown", onPointerDown);
    return () => window.removeEventListener("pointerdown", onPointerDown);
  }, [showPicker]);

  const blocked = status === "running" || status === "waiting_approval" || status === "waiting_question";

  const canSend = isChatMode
    ? !!selectedProvider && !blocked && text.trim().length > 0 && !showPicker
    : (launchMode === "workflow" ? !!selectedWorkflowId : !!selectedStepId) &&
      !blocked &&
      text.trim().length > 0;

  // Keep the skill picker multi-select active; the controller strip visually
  // downplays the other options but still reflects the current runtime state.
  const pickSkill = (name: string) => {
    setSelectedSkills((prev) => (prev.includes(name) ? prev : [...prev, name]));
  };

  const removeSkill = (name: string) => {
    setSelectedSkills((prev) => prev.filter((s) => s !== name));
  };

  const send = () => {
    if (!canSend) return;
    void sendPrompt(text.trim(), isChatMode ? selectedSkills : undefined);
    setText("");
    setSelectedSkills([]);
    setSkillPickerOpen(false);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !showPicker) {
      e.preventDefault();
      send();
    }
  };

  const placeholder = blocked
    ? "Waiting for the current turn…"
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
    <div ref={rootRef} className="chat-input">
      {isChatMode && (
        <div className={`chat-controller ${controllerExpanded ? "expanded" : "collapsed"}`}>
          <div className="chat-controller-head">
            <div className="chat-controller-head-copy">
              <span className="chat-controller-label">Chat controls</span>
              <strong>Provider, skills, and run settings</strong>
            </div>
            <button
              type="button"
              className="chat-controller-toggle"
              onClick={() => setControllerExpanded((current) => !current)}
              aria-expanded={controllerExpanded}
              aria-label={controllerExpanded ? "Collapse chat controls" : "Expand chat controls"}
            >
              <span aria-hidden="true">{controllerExpanded ? "▾" : "▸"}</span>
            </button>
          </div>

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
                    disabled={!selectedProvider}
                    aria-label={
                      selectedSkills.length > 0
                        ? `Select skills, ${selectedSkills.length} selected`
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
                        : selectedSkills.length > 0
                          ? `Select skills (${selectedSkills.length})`
                          : "Select skills"}
                    </span>
                    <span className="skill-select-caret" aria-hidden="true">
                      ▾
                    </span>
                  </button>
                </div>

                <div className="chat-controller-switch chat-controller-switch-top">
                  <span className="chat-controller-label">YOLO</span>
                  <button
                    type="button"
                    role="switch"
                    aria-checked={yoloMode}
                    className={`yolo-toggle ${yoloMode ? "active" : ""}`}
                    onClick={() => setYoloMode(!yoloMode)}
                  >
                    <span className="yolo-toggle-track" aria-hidden="true">
                      <span className="yolo-toggle-thumb" />
                    </span>
                    <span className="yolo-toggle-label">{yoloMode ? "On" : "Off"}</span>
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
                    />
                  )}
                </label>

                <label className="chat-controller-field muted">
                  <span>Reasoning</span>
                  <select
                    value={reasoningEffort ?? ""}
                    onChange={(e) => setReasoningEffort(e.target.value || undefined)}
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
                <span className="skill-name">/{s.name}</span>
                {s.description && <span className="skill-desc">{s.description}</span>}
                <span className={`skill-src src-${s.source}`}>{s.source}</span>
              </button>
            );
          })}
        </div>
      )}

      <div className="input-bar">
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
    </div>
  );
}
