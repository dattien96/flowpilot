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
  const [providers, setProviders] = useState<LocalRunnerProvider[]>([]);
  const [models, setModels] = useState<SupportedModel[]>([]);
  const [busy, setBusy] = useState(false);
  const [connectingProviderKey, setConnectingProviderKey] = useState<ProviderKey | null>(null);
  const [pendingProviderKey, setPendingProviderKey] = useState<ProviderKey | null>(null);
  const [pendingKnownAccountIds, setPendingKnownAccountIds] = useState<string[]>([]);
  const [message, setMessage] = useState<string | null>(null);
  const [messageTone, setMessageTone] = useState<"info" | "error">("info");
  const [draft, setDraft] = useState<ProviderDraft>({ providerKey: "codex", modelId: "", displayName: "" });

  const refresh = async () => {
    try {
      const admin = await getAdminUseCases();
      const [nextProviders, nextModels] = await Promise.all([
        admin.providers.listLocalProviders(),
        admin.providers.listSupportedModels(),
      ]);
      setProviders(nextProviders);
      setModels(nextModels);
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

  const authenticate = async (providerKey: string) => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      await admin.providers.authenticateProvider(providerKey);
      setMessageTone("info");
      setMessage(`Authentication started for ${providerKey}.`);
      await refresh();
    } catch (error) {
      setMessageTone("error");
      setMessage(toErrorMessage(error, "Unable to authenticate provider."));
    } finally {
      setBusy(false);
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
        <div className="settings-subpanel"><h3>Local Providers</h3><div className="settings-list">{providers.map((provider) => <div className="settings-list-item static" key={provider.key}><div className="settings-provider-meta"><strong>{provider.label}</strong><span>{provider.installed ? "installed" : "not installed"} / {provider.version ?? "unknown"}</span><span>{providerAccountSummary(provider.key as ProviderKey)}</span></div><div className="settings-provider-actions"><button className="secondary-btn" disabled={busy || connectingProviderKey === provider.key || !provider.installed} onClick={() => void connectAccount(provider.key as ProviderKey)} type="button">{connectingProviderKey === provider.key || pendingProviderKey === provider.key ? "Connecting..." : "Connect New Account"}</button><button className="secondary-btn" disabled={busy} onClick={() => void authenticate(provider.key)} type="button">Auth</button></div></div>)}</div></div>
        <div className="settings-subpanel"><h3>Add Supported Model</h3><div className="settings-grid"><label className="settings-field"><span>Provider</span><select value={draft.providerKey} onChange={(event) => setDraft((current) => ({ ...current, providerKey: event.target.value as "codex" | "claude" | "gemini" }))}><option value="codex">codex</option><option value="claude">claude</option><option value="gemini">gemini</option></select></label><label className="settings-field"><span>Model ID</span><input value={draft.modelId} onChange={(event) => setDraft((current) => ({ ...current, modelId: event.target.value }))} /></label><label className="settings-field settings-field-full"><span>Display Name</span><input value={draft.displayName} onChange={(event) => setDraft((current) => ({ ...current, displayName: event.target.value }))} /></label></div><div className="settings-actions"><button className="primary-btn" disabled={busy} onClick={() => void addModel()} type="button">Add Model</button></div></div>
      </div>
      <div className="settings-subpanel"><h3>Supported Models</h3><div className="settings-list">{models.map((model) => <div className="settings-list-item static" key={model.id}><div><strong>{model.displayName}</strong><span>{model.providerKey} / {model.modelId} / {model.isEnabled ? "enabled" : "disabled"}</span></div><button className="secondary-btn" disabled={busy} onClick={() => void toggleModel(model)} type="button">{model.isEnabled ? "Disable" : "Enable"}</button></div>)}</div></div>
    </section>
  );
}
