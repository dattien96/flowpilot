import { useState } from "react";
import type {
  SupabaseConfigInput,
  SupabaseConfigValidation,
  SupabaseRuntimeStatus,
} from "@flowpilot/client-core";

interface SupabaseSetupScreenProps {
  busy: boolean;
  runtimeStatus: SupabaseRuntimeStatus;
  onBack?: () => void;
  onValidate: (input: SupabaseConfigInput) => Promise<SupabaseConfigValidation>;
  onSave: (input: SupabaseConfigInput) => Promise<void>;
}

function suggestEdgeUrl(apiUrl: string) {
  const trimmed = apiUrl.trim().replace(/\/+$/, "");
  return trimmed ? `${trimmed}/functions/v1` : "";
}

export function SupabaseSetupScreen({
  busy,
  runtimeStatus,
  onBack,
  onValidate,
  onSave,
}: SupabaseSetupScreenProps): React.ReactElement {
  const [form, setForm] = useState<SupabaseConfigInput>({
    apiUrl: runtimeStatus.apiUrl ?? "",
    anonKey: runtimeStatus.anonKey ?? "",
    edgeFunctionUrl: runtimeStatus.edgeFunctionUrl ?? "",
    serviceRoleKey: "",
  });
  const [edgeTouched, setEdgeTouched] = useState(Boolean(runtimeStatus.edgeFunctionUrl));
  const [validation, setValidation] = useState<SupabaseConfigValidation | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);

  const updateApiUrl = (apiUrl: string) => {
    setForm((current) => ({
      ...current,
      apiUrl,
      edgeFunctionUrl: edgeTouched ? current.edgeFunctionUrl : suggestEdgeUrl(apiUrl),
    }));
    setValidation(null);
  };

  const updateField = (key: keyof SupabaseConfigInput, value: string) => {
    if (key === "edgeFunctionUrl") {
      setEdgeTouched(true);
    }
    setForm((current) => ({ ...current, [key]: value }));
    setValidation(null);
  };

  const handleValidate = async () => {
    setFeedback(null);
    try {
      const result = await onValidate(form);
      setValidation(result);
      setFeedback(result.valid ? "Connection checks passed." : "Fix the failed checks before saving.");
    } catch (error) {
      setFeedback(error instanceof Error ? error.message : "Connection test failed.");
    }
  };

  const handleSave = async () => {
    setFeedback(null);
    try {
      await onSave(form);
      setFeedback("Supabase config saved.");
    } catch (error) {
      setFeedback(error instanceof Error ? error.message : "Unable to save Supabase config.");
    }
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head">
        <div>
          <div className="settings-eyebrow">Workspace Runtime</div>
          <h2>Supabase</h2>
          <p>
            Configure the database and storage connection used by desktop login and
            shared app data.
          </p>
        </div>
        <div className={`runner-pill ${runtimeStatus.runnerReachable ? "online" : "offline"}`}>
          {runtimeStatus.runnerReachable ? "Runner online" : "Runner offline"}
        </div>
      </div>

      <div className="settings-grid">
        <label className="settings-field">
          <span>Supabase URL</span>
          <input
            disabled={busy}
            onChange={(event) => updateApiUrl(event.target.value)}
            placeholder="https://project-ref.supabase.co"
            value={form.apiUrl}
          />
        </label>

        <label className="settings-field">
          <span>Anon Key</span>
          <input
            disabled={busy}
            onChange={(event) => updateField("anonKey", event.target.value)}
            placeholder="Public anon key"
            value={form.anonKey}
          />
        </label>

        <label className="settings-field">
          <span>Edge Function URL</span>
          <input
            disabled={busy}
            onChange={(event) => updateField("edgeFunctionUrl", event.target.value)}
            placeholder="https://project-ref.supabase.co/functions/v1"
            value={form.edgeFunctionUrl}
          />
        </label>

        <label className="settings-field">
          <span>Service Role Key</span>
          <input
            disabled={busy}
            onChange={(event) => updateField("serviceRoleKey", event.target.value)}
            placeholder={runtimeStatus.hasServiceRoleKey ? "Leave blank to keep existing key" : "Privileged service role key"}
            value={form.serviceRoleKey}
          />
        </label>
      </div>

      {feedback ? <div className="settings-feedback">{feedback}</div> : null}

      {validation ? (
        <div className="settings-validation">
          {validation.checks.map((check) => (
            <div className={`validation-row ${check.status}`} key={check.key}>
              <span>{check.key}</span>
              <span>{check.message}</span>
            </div>
          ))}
        </div>
      ) : null}

      <div className="settings-actions">
        <button className="secondary-btn" disabled={busy || !runtimeStatus.runnerReachable} onClick={() => void handleValidate()} type="button">
          {busy ? "Working..." : "Test Connection"}
        </button>
        <button
          className="primary-btn"
          disabled={busy || !runtimeStatus.runnerReachable || validation?.valid !== true}
          onClick={() => void handleSave()}
          type="button"
        >
          Save Config
        </button>
        {onBack ? (
          <button className="ghost-btn" onClick={onBack} type="button">
            Back
          </button>
        ) : null}
      </div>
    </section>
  );
}
