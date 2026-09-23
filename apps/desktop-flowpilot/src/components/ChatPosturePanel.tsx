import { useEffect, useMemo, useState } from "react";
import { useStore } from "@/state/store";
import { CHAT_POSTURES, type ChatPosture, type ChatPostureConfig, type ChatPostureProfile } from "@/types/contract";
import {
  applyPostureModelPick,
  buildPostureModelGroups,
  DEFAULT_POSTURE_REASONING_OPTIONS,
  postureReasoningOptions,
} from "./postureModelPicker";

// ChatPosturePanel (Task-xxx/CA-xxx): OpenCode-style Scan/Plan/Code mode
// switching. A tab strip (mirroring ChatStartIntentPanel's chat-start-mode-tab)
// that activates a posture and applies its pinned profile; the ⚙ button opens a
// setup modal that edits each posture's profile (provider/model/reasoning/yolo)
// and persists via the runner's PUT /client/chat-posture (SSOT).

function PostureModeIcon({ posture }: { posture: ChatPosture }): React.ReactElement {
  if (posture === "scan") {
    return (
      <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
        <circle cx="7" cy="7" r="3.6" fill="none" stroke="currentColor" strokeWidth="1.4" />
        <path d="M9.7 9.7 13 13" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
      </svg>
    );
  }
  if (posture === "non") {
    return (
      <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
        <circle cx="8" cy="8" r="5.2" fill="none" stroke="currentColor" strokeWidth="1.3" />
        <path d="M5.4 8h5.2" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
      </svg>
    );
  }
  if (posture === "plan") {
    return (
      <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
        <path d="M8 2v2M8 12v2M2 8h2M12 8h2M4.4 4.4l1.4 1.4M10.2 10.2l1.4 1.4M11.6 4.4l-1.4 1.4M5.8 10.2l-1.4 1.4" fill="none" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
        <circle cx="8" cy="8" r="2" fill="none" stroke="currentColor" strokeWidth="1.3" />
      </svg>
    );
  }
  return (
    <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
      <path d="M3 3l10 10M13 3L3 13" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
      <rect x="5.5" y="5.5" width="5" height="5" rx="1" fill="none" stroke="currentColor" strokeWidth="1.3" />
    </svg>
  );
}

// RunInWorktreeToggle (CP-71 / Task-409 T-1): opt the next run into an
// isolated git worktree. Disabled reasons (T-3): non-git project, live
// binding (must merge/discard first), or a turn in flight.
export function RunInWorktreeToggle(props: {
  enabled: boolean;
  disabled: boolean;
  disabledReason?: string;
  onToggle(on: boolean): void;
}): React.ReactElement {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={props.enabled}
      aria-label="Run in worktree"
      className={`worktree-toggle ${props.enabled ? "active" : ""}`}
      disabled={props.disabled}
      title={props.disabledReason ?? "Run the next turn in an isolated git worktree"}
      onClick={() => props.onToggle(!props.enabled)}
    >
      <svg viewBox="0 0 16 16" width="13" height="13" aria-hidden="true">
        <circle cx="5" cy="4" r="2" fill="none" stroke="currentColor" strokeWidth="1.3" />
        <circle cx="5" cy="12" r="2" fill="none" stroke="currentColor" strokeWidth="1.3" />
        <circle cx="11" cy="8" r="2" fill="none" stroke="currentColor" strokeWidth="1.3" />
        <path d="M5 6v4M6.7 5.4C8 6 9.5 6.5 10.5 6.9" fill="none" stroke="currentColor" strokeWidth="1.2" />
      </svg>
      <span className="worktree-toggle-label">Worktree</span>
      <span className="worktree-toggle-state">{props.enabled ? "On" : "Off"}</span>
    </button>
  );
}

export function ChatPosturePanel(): React.ReactElement | null {
  const chatMode = useStore((s) => s.chatMode);
  const chatPosture = useStore((s) => s.chatPosture);
  const setChatPosture = useStore((s) => s.setChatPosture);
  const chatPostureSetupOpen = useStore((s) => s.chatPostureSetupOpen);
  const openChatPostureSetup = useStore((s) => s.openChatPostureSetup);
  const closeChatPostureSetup = useStore((s) => s.closeChatPostureSetup);
  const runStatus = useStore((s) => s.status);
  const worktreeEnabled = useStore((s) => s.worktreeEnabled);
  const worktreeAvailable = useStore((s) => s.worktreeAvailable);
  const activeWorktreeState = useStore((s) => s.activeWorktreeState);
  const setWorktreeEnabled = useStore((s) => s.setWorktreeEnabled);

  // BUG-NOTE: posture is resendable between prompts like model/YOLO (the runner
  // applies ChatPosture per turn), so unlike ChatStartIntentPanel it is NOT
  // locked once the chat starts — only disabled while a turn is in flight.
  const isRunning = runStatus === "running";

  useEffect(() => {
    // Fetch the runner-owned posture document once the runner is reachable so
    // the tabs reflect the SSOT (TUI + Desktop share it). No-op on failure.
    void useStore.getState().loadChatPostureConfig();
  }, []);

  if (chatMode !== "normal_chat") return null;

  return (
    <section className="workflow-rail workflow-rail-right chat-posture-panel">
      <div className="project-rail-head">
        <div>
          <label>Posture</label>
          <p>Scan/Plan are read-only — writes auto-deny without asking.</p>
        </div>
        <button
          type="button"
          className="chat-posture-gear"
          aria-label="Configure posture profiles"
          title="Configure posture profiles"
          onClick={openChatPostureSetup}
        >
          <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
            <circle cx="8" cy="8" r="2.4" fill="none" stroke="currentColor" strokeWidth="1.3" />
            <path d="M8 1.8v2M8 12.2v2M1.8 8h2M12.2 8h2M3.6 3.6l1.4 1.4M11 11l1.4 1.4M12.4 3.6 11 5M5 11l-1.4 1.4" fill="none" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
          </svg>
        </button>
      </div>
      <div className="tab-list tab-list-four" role="tablist" aria-label="Chat posture">
        {CHAT_POSTURES.map((item) => (
          <button
            key={item.key}
            type="button"
            role="tab"
            aria-selected={chatPosture === item.key}
            className={`tab chat-posture-tab is-${item.key} ${chatPosture === item.key ? "active" : ""}`}
            disabled={isRunning}
            title={item.hint}
            onClick={() => void setChatPosture(item.key)}
          >
            <span className="chat-posture-icon"><PostureModeIcon posture={item.key} /></span>
            <span>{item.label}</span>
          </button>
        ))}
      </div>
      <p className="chat-posture-hint">
        {CHAT_POSTURES.find((item) => item.key === chatPosture)?.hint}
      </p>
      <RunInWorktreeToggle
        enabled={worktreeEnabled}
        disabled={
          isRunning ||
          !worktreeAvailable ||
          (worktreeEnabled && (activeWorktreeState === "active" || activeWorktreeState === "merge_pending"))
        }
        disabledReason={
          !worktreeAvailable
            ? "Run in worktree requires a git repository"
            : worktreeEnabled && (activeWorktreeState === "active" || activeWorktreeState === "merge_pending")
              ? "Merge or discard the worktree first"
              : undefined
        }
        onToggle={setWorktreeEnabled}
      />
      {chatPostureSetupOpen ? (
        <ChatPostureSetupModal
          posture={chatPosture}
          onClose={closeChatPostureSetup}
        />
      ) : null}
    </section>
  );
}

// ChatPostureSetupModal edits each posture's pinned profile and saves it to the
// runner. Empty fields inherit the current session selection. The modal uses a
// 3-tab layout (Scan | Plan | Code) so only one profile is visible at a time.
function ChatPostureSetupModal({ posture, onClose }: { posture: ChatPosture; onClose: () => void }): React.ReactElement {
  const config = useStore((s) => s.chatPostureConfig);
  const saveChatPostureConfig = useStore((s) => s.saveChatPostureConfig);
  const localProviders = useStore((s) => s.localProviders);
  const supportedModels = useStore((s) => s.supportedModels);
  const [modalTab, setModalTab] = useState<ChatPosture>(posture);
  const [draft, setDraft] = useState<ChatPostureConfig>(() => ({
    active: config.active,
    profiles: {
      scan: { ...config.profiles.scan },
      plan: { ...config.profiles.plan },
      code: { ...config.profiles.code },
    },
  }));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const updateProfile = (key: ChatPosture, patch: Partial<ChatPostureProfile>) => {
    setDraft((prev) => ({
      ...prev,
      profiles: {
        ...prev.profiles,
        [key]: { ...prev.profiles[key], ...patch },
      },
    }));
  };

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      await saveChatPostureConfig(draft);
      onClose();
    } catch (err) {
      setError(String(err));
    } finally {
      setSaving(false);
    }
  };

  const activeProf = draft.profiles[modalTab] ?? {};
  const pinActive = Boolean(activeProf.provider || activeProf.model || activeProf.reasoningEffort || typeof activeProf.yolo === "boolean");
  // BUG-348: no provider picker (TUI parity) — the provider is always
  // auto-inferred from the picked model. The list is preloaded from the
  // supported registry + runner-detected models, grouped by provider.
  const modelGroups = useMemo(
    () => buildPostureModelGroups(supportedModels, localProviders, activeProf.model, activeProf.provider),
    [supportedModels, localProviders, activeProf.model, activeProf.provider],
  );
  const reasoningOpts = postureReasoningOptions(supportedModels, activeProf.model ?? "");
  const reasoningLabels: Record<string, string> = {
    minimal: "Minimal",
    low: "Low",
    medium: "Medium",
    high: "High",
    xhigh: "Extra High",
    max: "Max",
  };

  const onModelChange = (value: string) => {
    const pick = applyPostureModelPick(supportedModels, localProviders, value);
    updateProfile(modalTab, {
      model: pick.modelId || undefined,
      provider: pick.provider,
      reasoningEffort: pick.reasoningEffort,
    });
  };

  return (
    <div className="account-switch-overlay" role="dialog" aria-modal="true" aria-label="Configure chat postures" onClick={onClose}>
      <div className="account-switch-modal chat-posture-modal" onClick={(e) => e.stopPropagation()}>
        <p className="account-switch-reason">Configure posture profiles</p>
        <p className="chat-posture-modal-hint">
          Each posture can pin a model/reasoning/YOLO (the provider
          auto-derives from the picked model). Empty fields inherit the
          current session selection. Scan/Plan are read-only (reads auto-approve, writes auto-deny).
        </p>
        <div className="tab-list tab-list-four chat-posture-modal-tabs" role="tablist" aria-label="Posture profiles">
          {CHAT_POSTURES.map((item) => (
            <button
              key={item.key}
              type="button"
              role="tab"
              aria-selected={modalTab === item.key}
              className={`tab chat-posture-modal-tab is-${item.key} ${modalTab === item.key ? "active" : ""}`}
              onClick={() => setModalTab(item.key)}
            >
              <span>{item.label}</span>
              {item.key === posture ? <span className="chat-posture-modal-tab-active-dot" /> : null}
            </button>
          ))}
        </div>
        <div className={`chat-posture-modal-profile is-${modalTab}`}>
          <label className="settings-field"><span>Model</span>
            <select
              value={activeProf.model ?? ""}
              onChange={(e) => onModelChange(e.target.value)}
            >
              <option value="">(inherit)</option>
              {modelGroups.map((g) => (
                <optgroup key={g.providerKey} label={g.providerKey}>
                  {g.options.map((o) => (
                    <option key={`${o.providerKey}/${o.modelId}`} value={o.modelId}>
                      {o.displayName}
                    </option>
                  ))}
                </optgroup>
              ))}
            </select>
          </label>
          <p className="chat-posture-modal-provider-note">
            Provider auto: {activeProf.provider ? activeProf.provider : "(inherit)"}
          </p>
          <label className="settings-field"><span>Reasoning</span>
            <select
              value={activeProf.reasoningEffort ?? ""}
              disabled={reasoningOpts === null}
              title={reasoningOpts === null ? "Pick a model first" : undefined}
              onChange={(e) => updateProfile(modalTab, { reasoningEffort: e.target.value || undefined })}
            >
              <option value="">(inherit)</option>
              {(reasoningOpts ?? [...DEFAULT_POSTURE_REASONING_OPTIONS]).map((opt) => (
                <option key={opt} value={opt}>{reasoningLabels[opt] ?? opt}</option>
              ))}
              {activeProf.reasoningEffort && !(reasoningOpts ?? [...DEFAULT_POSTURE_REASONING_OPTIONS]).includes(activeProf.reasoningEffort) ? (
                <option value={activeProf.reasoningEffort}>{activeProf.reasoningEffort}</option>
              ) : null}
            </select>
          </label>
          <label className="settings-field"><span>YOLO</span>
            <select
              value={typeof activeProf.yolo === "boolean" ? (activeProf.yolo ? "on" : "off") : ""}
              onChange={(e) => {
                const v = e.target.value;
                updateProfile(modalTab, { yolo: v === "" ? undefined : v === "on" });
              }}
            >
              <option value="">(inherit)</option>
              <option value="on">On</option>
              <option value="off">Off</option>
            </select>
          </label>
          {pinActive ? (
            <button type="button" className="chat-posture-clear" onClick={() => updateProfile(modalTab, { provider: undefined, model: undefined, reasoningEffort: undefined, yolo: undefined })}>
              Clear pins
            </button>
          ) : null}
        </div>
        {error ? <p className="chat-posture-modal-error">{error}</p> : null}
        <div className="account-switch-actions">
          <button type="button" className="project-history-confirm-cancel" onClick={onClose} disabled={saving}>
            Cancel
          </button>
          <button type="button" className="project-history-confirm-ok" onClick={() => void save()} disabled={saving}>
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}