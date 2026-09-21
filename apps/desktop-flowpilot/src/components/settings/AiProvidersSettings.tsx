import { useEffect, useState } from "react";
import type { LocalRunnerProvider, SupportedModel } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { toErrorMessage } from "@/components/settings/settingsHelpers";
import { useStore } from "@/state/store";
import type { ProviderKey } from "@/types/contract";

type ProviderDraft = {
  providerKey: ProviderKey;
  modelId: string;
  displayName: string;
};

export function AiProvidersSettings(): React.ReactElement {
  const client = useStore((state) => state.client);
  const providerAccounts = useStore((state) => state.providerAccounts);
  const loadProviderAccounts = useStore((state) => state.loadProviderAccounts);
  const loadLocalProviders = useStore((state) => state.loadLocalProviders);
  const [providers, setProviders] = useState<LocalRunnerProvider[]>([]);
  const [models, setModels] = useState<SupportedModel[]>([]);
  const [busy, setBusy] = useState(false);
  const [connectingProviderKey, setConnectingProviderKey] = useState<ProviderKey | null>(null);
  const [installingProviderKey, setInstallingProviderKey] = useState<ProviderKey | null>(null);
  const [pendingProviderKey, setPendingProviderKey] = useState<ProviderKey | null>(null);
  const [pendingKnownAccountIds, setPendingKnownAccountIds] = useState<string[]>([]);
  const [message, setMessage] = useState<string | null>(null);
  const [messageTone, setMessageTone] = useState<"info" | "error">("info");
  const [draft, setDraft] = useState<ProviderDraft>({ providerKey: "codex", modelId: "", displayName: "" });
  const [editingModelId, setEditingModelId] = useState<string | null>(null);
  const [editDraft, setEditDraft] = useState<{ modelId: string; displayName: string }>({ modelId: "", displayName: "" });
  // Task-213: per-provider "Detect models" sync state.
  const [detectingProviderKey, setDetectingProviderKey] = useState<ProviderKey | null>(null);
  const [detectSummary, setDetectSummary] = useState<Record<string, string>>({});
  const [detectError, setDetectError] = useState<Record<string, string>>({});

  const refresh = async () => {
    try {
      const admin = await getAdminUseCases();
      const [nextProviders, nextModels] = await Promise.all([
        admin.providers.listLocalProviders(),
        admin.providers.listSupportedModels(),
      ]);
      setProviders(nextProviders);
      setModels(nextModels);
      await loadLocalProviders();
    } catch (error) {
      setMessageTone("error");
      setMessage(toErrorMessage(error, "Unable to load AI providers."));
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    void loadProviderAccounts();
  }, [loadProviderAccounts]);

  useEffect(() => {
    if (!pendingProviderKey) return;
    const intervalId = window.setInterval(() => {
      void loadProviderAccounts();
    }, 2000);
    return () => window.clearInterval(intervalId);
  }, [loadProviderAccounts, pendingProviderKey]);

  useEffect(() => {
    if (!pendingProviderKey) return;
    const nextAccounts = providerAccounts.filter((account) => account.providerKey === pendingProviderKey);
    const hasNewAccount = nextAccounts.some((account) => !pendingKnownAccountIds.includes(account.id));
    if (!hasNewAccount) return;
    setPendingProviderKey(null);
    setPendingKnownAccountIds([]);
    setMessageTone("info");
    setMessage("New provider account detected. Desktop settings refreshed automatically.");
  }, [pendingKnownAccountIds, pendingProviderKey, providerAccounts]);

  const addModel = async () => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      await admin.providers.createSupportedModel({
        ...draft,
        isEnabled: true,
        sortOrder: models.reduce((max, model) => Math.max(max, model.sortOrder), 0) + 1,
        source: "manual",
        detectionMethod: null,
        detectedCliVersion: null,
        lastDetectedAt: null,
        supportedReasoningEfforts: null,
        defaultReasoningEffort: null,
        contextWindowTokens: null,
        maxContextWindowTokens: null,
        inputImage: null,
      });
      setDraft({ providerKey: "codex", modelId: "", displayName: "" });
      await refresh();
      setMessageTone("info");
      setMessage("Supported model added.");
    } catch (error) {
      setMessageTone("error");
      setMessage(toErrorMessage(error, "Unable to add supported model."));
    } finally {
      setBusy(false);
    }
  };

  // Task-213: pull the runner's live-detected model list for one provider
  // (already loaded into `providers[].models` from GET /providers — no new
  // HTTP call) and insert whichever ones aren't yet in the ai_supported_models
  // catalog. Never touches an existing row's model identity/user edits
  // (skip-if-registered for those fields), mirroring apps/admin-web's "Check
  // and import missing models" action.
  //
  // Task-215: additionally stamps/refreshes each model's detected reasoning-
  // effort support and context-window size — for a newly-inserted row as
  // part of the create, and for an already-registered row via a targeted
  // update (unlike model identity, these are capability facts about the
  // model, not user-editable state, so re-detecting is free to refresh them).
  const detectModels = async (providerKey: ProviderKey) => {
    setDetectingProviderKey(providerKey);
    setDetectError((prev) => {
      const next = { ...prev };
      delete next[providerKey];
      return next;
    });
    try {
      const provider = providers.find((p) => p.key === providerKey);
      const detected = provider?.models ?? [];
      if (detected.length === 0) {
        setDetectSummary((prev) => ({
          ...prev,
          [providerKey]: "No models detected — is the CLI installed and authenticated?",
        }));
        return;
      }

      const registeredByModelId = new Map(
        models.filter((model) => model.providerKey === providerKey).map((model) => [model.modelId, model] as const),
      );
      const maxSortOrder = models.reduce((max, model) => Math.max(max, model.sortOrder), 0);

      const admin = await getAdminUseCases();
      const lastDetectedAt = new Date().toISOString();
      let importedCount = 0;
      let refreshedCount = 0;
      for (const detectedModel of detected) {
        const reasoningPatch = {
          supportedReasoningEfforts: detectedModel.supportedReasoningEfforts ?? null,
          defaultReasoningEffort: detectedModel.defaultReasoningEffort ?? null,
          contextWindowTokens: detectedModel.contextWindowTokens ?? null,
          maxContextWindowTokens: detectedModel.maxContextWindowTokens ?? null,
          inputImage: detectedModel.inputImage ?? null,
        };
        const existing = registeredByModelId.get(detectedModel.id);
        if (existing) {
          await admin.providers.updateSupportedModel(existing.id, reasoningPatch);
          refreshedCount++;
          continue;
        }
        const created = await admin.providers.createSupportedModel({
          providerKey,
          modelId: detectedModel.id,
          displayName: detectedModel.displayName || detectedModel.id,
          isEnabled: true,
          sortOrder: maxSortOrder + importedCount + 1,
          source: "detected",
          detectionMethod: detectedModel.source || null,
          detectedCliVersion: provider?.detectedVersion ?? null,
          lastDetectedAt,
          ...reasoningPatch,
        });
        registeredByModelId.set(detectedModel.id, created);
        importedCount++;
      }

      setDetectSummary((prev) => ({
        ...prev,
        [providerKey]: `${importedCount} new model(s) imported, ${refreshedCount} refreshed with latest reasoning/context data.`,
      }));
      await refresh();
    } catch (error) {
      setDetectError((prev) => ({
        ...prev,
        [providerKey]: toErrorMessage(error, "Unable to detect models."),
      }));
    } finally {
      setDetectingProviderKey(null);
    }
  };

  const deleteModel = async (model: SupportedModel) => {
    if (!window.confirm(`Delete "${model.displayName}"?`)) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      await admin.providers.deleteSupportedModel(model.id);
      await refresh();
    } catch (error) {
      setMessageTone("error");
      setMessage(toErrorMessage(error, "Unable to delete supported model."));
    } finally {
      setBusy(false);
    }
  };

  const saveEditModel = async () => {
    if (!editingModelId) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const updated = await admin.providers.updateSupportedModel(editingModelId, {
        modelId: editDraft.modelId.trim(),
        displayName: editDraft.displayName.trim(),
      });
      setModels((current) => current.map((m) => (m.id === updated.id ? updated : m)));
      setEditingModelId(null);
    } catch (error) {
      setMessageTone("error");
      setMessage(toErrorMessage(error, "Unable to update supported model."));
    } finally {
      setBusy(false);
    }
  };

  const toggleModel = async (model: SupportedModel) => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      await admin.providers.updateSupportedModel(model.id, { isEnabled: !model.isEnabled });
      await refresh();
    } catch (error) {
      setMessageTone("error");
      setMessage(toErrorMessage(error, "Unable to update supported model."));
    } finally {
      setBusy(false);
    }
  };

  const installProvider = async (providerKey: ProviderKey) => {
    setInstallingProviderKey(providerKey);
    setMessageTone("info");
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const nextProviders = await admin.providers.installLocalProvider(providerKey);
      setProviders(nextProviders);
      await loadLocalProviders();
      setMessage(`${providerKey === "gemini" ? "AGY CLI" : providerKey === "opencode" ? "OpenCode CLI" : providerKey} install requested. Refresh or connect an account after the installer completes.`);
    } catch (error) {
      setMessageTone("error");
      setMessage(toErrorMessage(error, "Unable to install provider CLI."));
    } finally {
      setInstallingProviderKey(null);
    }
  };

  const connectAccount = async (providerKey: ProviderKey) => {
    setConnectingProviderKey(providerKey);
    setMessageTone("info");
    setMessage(null);
    try {
      const knownAccountIds = providerAccounts
        .filter((account) => account.providerKey === providerKey)
        .map((account) => account.id);
      await client.connectProviderAccount(providerKey);
      setPendingKnownAccountIds(knownAccountIds);
      setPendingProviderKey(providerKey);
      setMessage("Login terminal requested. Complete authentication in the opened terminal/browser. Desktop settings will refresh automatically when the new account is detected.");
      await loadProviderAccounts();
    } catch (error) {
      setPendingProviderKey(null);
      setPendingKnownAccountIds([]);
      setMessageTone("error");
      setMessage(toErrorMessage(error, "Unable to connect provider account."));
    } finally {
      setConnectingProviderKey(null);
    }
  };

  const providerAccountSummary = (providerKey: ProviderKey): string => {
    const accounts = providerAccounts.filter((account) => account.providerKey === providerKey);
    const connectedCount = accounts.filter((account) => account.authStatus === "connected").length;
    const activeAccount = accounts.find((account) => account.isActive) ?? null;
    if (accounts.length === 0) return "No accounts connected.";
    const parts = [`${accounts.length} account(s)`, `${connectedCount} connected`];
    if (activeAccount) {
      parts.push(`active: ${activeAccount.displayLabel}`);
    }
    return parts.join(" / ");
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head"><div><div className="settings-eyebrow">AI Providers</div><h2>AI Providers</h2><p>Inspect local provider readiness and manage supported models.</p></div></div>
      {message ? <div className={`settings-feedback${messageTone === "error" ? " error" : ""}`}>{message}</div> : null}
      <div className="settings-two-column">
        <div className="settings-subpanel">
          <h3>Local Providers</h3>
          <div className="settings-list">
            {providers.map((provider) => (
              <div className="settings-list-item static" key={provider.key}>
                <div className="settings-provider-meta">
                  <strong>{provider.label}</strong>
                  <span>{provider.installed ? "installed" : "not installed"} / {provider.detectedVersion ?? provider.version ?? "unknown"}</span>
                  <span>{providerAccountSummary(provider.key as ProviderKey)}</span>
                  {detectSummary[provider.key] ? <span className="settings-feedback">{detectSummary[provider.key]}</span> : null}
                  {detectError[provider.key] ? <span className="settings-feedback error">{detectError[provider.key]}</span> : null}
                </div>
                <div className="settings-provider-actions">
                  {provider.key === "gemini" || provider.key === "opencode" || provider.key === "devin" ? (
                    <button className="secondary-btn" disabled={busy || installingProviderKey === provider.key || provider.installed} onClick={() => void installProvider(provider.key as ProviderKey)} type="button">
                      {installingProviderKey === provider.key ? `Installing ${provider.key === "gemini" ? "AGY" : provider.key === "opencode" ? "OpenCode" : "Devin"}...` : provider.installed ? `${provider.key === "gemini" ? "AGY" : provider.key === "opencode" ? "OpenCode" : "Devin"} Installed` : `Install ${provider.key === "gemini" ? "AGY CLI" : provider.key === "opencode" ? "OpenCode CLI" : "Devin CLI"}`}
                    </button>
                  ) : null}
                  <button className="secondary-btn" disabled={busy || connectingProviderKey === provider.key || !provider.installed} onClick={() => void connectAccount(provider.key as ProviderKey)} type="button">
                    {connectingProviderKey === provider.key || pendingProviderKey === provider.key ? "Connecting..." : "Connect New Account"}
                  </button>
                  {/* Task-213: pull the runner's live-detected model list into the catalog below. */}
                  <button className="secondary-btn" disabled={busy || detectingProviderKey === provider.key || !provider.installed} onClick={() => void detectModels(provider.key as ProviderKey)} type="button">
                    {detectingProviderKey === provider.key ? "Detecting..." : "Detect models"}
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
        <div className="settings-subpanel"><h3>Add Supported Model</h3><div className="settings-grid"><label className="settings-field"><span>Provider</span><select value={draft.providerKey} onChange={(event) => setDraft((current) => ({ ...current, providerKey: event.target.value as "codex" | "claude" | "gemini" | "grok" | "opencode" | "devin" }))}><option value="codex">codex</option><option value="claude">claude</option><option value="gemini">gemini</option><option value="grok">grok</option><option value="opencode">opencode</option><option value="devin">devin</option></select></label><label className="settings-field"><span>Model ID</span><input value={draft.modelId} onChange={(event) => setDraft((current) => ({ ...current, modelId: event.target.value }))} /></label><label className="settings-field settings-field-full"><span>Display Name</span><input value={draft.displayName} onChange={(event) => setDraft((current) => ({ ...current, displayName: event.target.value }))} /></label></div><div className="settings-actions"><button className="primary-btn" disabled={busy} onClick={() => void addModel()} type="button">Add Model</button></div></div>
      </div>
      <div className="settings-subpanel">
        <h3>Supported Models</h3>
        <div className="settings-list">
          {models.map((model) => {
            const isEditing = editingModelId === model.id;
            if (isEditing) {
              return (
                <div className="settings-list-item static" key={model.id}>
                  <div className="settings-grid" style={{ flex: 1, marginRight: "0.5rem" }}>
                    <label className="settings-field">
                      <span>Display Name</span>
                      <input
                        value={editDraft.displayName}
                        disabled={busy}
                        onChange={(e) => setEditDraft((d) => ({ ...d, displayName: e.target.value }))}
                      />
                    </label>
                    <label className="settings-field">
                      <span>Model ID</span>
                      <input
                        value={editDraft.modelId}
                        disabled={busy}
                        onChange={(e) => setEditDraft((d) => ({ ...d, modelId: e.target.value }))}
                      />
                    </label>
                  </div>
                  <div style={{ display: "flex", gap: "0.5rem", alignSelf: "center" }}>
                    <button className="primary-btn" disabled={busy || !editDraft.modelId.trim() || !editDraft.displayName.trim()} onClick={() => void saveEditModel()} type="button">Save</button>
                    <button className="secondary-btn" disabled={busy} onClick={() => setEditingModelId(null)} type="button">Cancel</button>
                  </div>
                </div>
              );
            }
            return (
              <div className="settings-list-item static" key={model.id}>
                <div>
                  <strong>{model.displayName}</strong>{" "}
                  {model.source === "detected" ? <span className="settings-badge">detected</span> : null}
                  <span>{model.providerKey} / {model.modelId} / {model.isEnabled ? "enabled" : "disabled"}</span>
                </div>
                <div style={{ display: "flex", gap: "0.5rem" }}>
                  <button className="secondary-btn" disabled={busy} onClick={() => void toggleModel(model)} type="button">{model.isEnabled ? "Disable" : "Enable"}</button>
                  <button className="secondary-btn" disabled={busy} onClick={() => { setEditDraft({ modelId: model.modelId, displayName: model.displayName }); setEditingModelId(model.id); }} type="button">Edit</button>
                  <button className="secondary-btn" disabled={busy} onClick={() => void deleteModel(model)} type="button">Delete</button>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
