import { useEffect, useState } from "react";
import type { LocalRunnerProvider, SupportedModel } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { toErrorMessage } from "@/components/settings/settingsHelpers";

export function AiProvidersSettings(): React.ReactElement {
  const [providers, setProviders] = useState<LocalRunnerProvider[]>([]);
  const [models, setModels] = useState<SupportedModel[]>([]);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [draft, setDraft] = useState({ providerKey: "codex" as "codex" | "claude" | "gemini", modelId: "", displayName: "" });

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
      setMessage(toErrorMessage(error, "Unable to load AI providers."));
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

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
      setMessage("Supported model added.");
    } catch (error) {
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
      setMessage(`Authentication started for ${providerKey}.`);
      await refresh();
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to authenticate provider."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head"><div><div className="settings-eyebrow">AI Providers</div><h2>AI Providers</h2><p>Inspect local provider readiness and manage supported models.</p></div></div>
      {message ? <div className="settings-feedback">{message}</div> : null}
      <div className="settings-two-column">
        <div className="settings-subpanel"><h3>Local Providers</h3><div className="settings-list">{providers.map((provider) => <div className="settings-list-item static" key={provider.key}><div><strong>{provider.label}</strong><span>{provider.installed ? "installed" : "not installed"} / {provider.version ?? "unknown"}</span></div><button className="secondary-btn" disabled={busy} onClick={() => void authenticate(provider.key)} type="button">Auth</button></div>)}</div></div>
        <div className="settings-subpanel"><h3>Add Supported Model</h3><div className="settings-grid"><label className="settings-field"><span>Provider</span><select value={draft.providerKey} onChange={(event) => setDraft((current) => ({ ...current, providerKey: event.target.value as "codex" | "claude" | "gemini" }))}><option value="codex">codex</option><option value="claude">claude</option><option value="gemini">gemini</option></select></label><label className="settings-field"><span>Model ID</span><input value={draft.modelId} onChange={(event) => setDraft((current) => ({ ...current, modelId: event.target.value }))} /></label><label className="settings-field settings-field-full"><span>Display Name</span><input value={draft.displayName} onChange={(event) => setDraft((current) => ({ ...current, displayName: event.target.value }))} /></label></div><div className="settings-actions"><button className="primary-btn" disabled={busy} onClick={() => void addModel()} type="button">Add Model</button></div></div>
      </div>
      <div className="settings-subpanel"><h3>Supported Models</h3><div className="settings-list">{models.map((model) => <div className="settings-list-item static" key={model.id}><div><strong>{model.displayName}</strong><span>{model.providerKey} / {model.modelId} / {model.isEnabled ? "enabled" : "disabled"}</span></div><button className="secondary-btn" disabled={busy} onClick={() => void toggleModel(model)} type="button">{model.isEnabled ? "Disable" : "Enable"}</button></div>)}</div></div>
    </section>
  );
}
