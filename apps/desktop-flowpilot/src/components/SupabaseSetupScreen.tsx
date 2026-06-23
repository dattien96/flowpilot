import { useState } from "react";
import type {
  SupabaseConfigInput,
  SupabaseConfigValidation,
  SupabaseSchemaApplyInput,
  SupabaseSchemaApplyResult,
  SupabaseRuntimeStatus,
} from "@flowpilot/client-core";

interface SupabaseSetupScreenProps {
  busy: boolean;
  runtimeStatus: SupabaseRuntimeStatus;
  onBack?: () => void;
  onValidate: (input: SupabaseConfigInput) => Promise<SupabaseConfigValidation>;
  onSave: (input: SupabaseConfigInput) => Promise<void>;
  onApplyMigrations: (
    input: SupabaseSchemaApplyInput,
  ) => Promise<SupabaseSchemaApplyResult>;
}

function suggestEdgeUrl(apiUrl: string) {
  const trimmed = apiUrl.trim().replace(/\/+$/, "");
  return trimmed ? `${trimmed}/functions/v1` : "";
}

function deriveProjectRef(apiUrl: string) {
  try {
    const host = new URL(apiUrl.trim()).host.toLowerCase();
    return host.replace(/\.supabase\.co$/, "").split(".")[0] || "";
  } catch {
    return "";
  }
}

export function SupabaseSetupScreen({
  busy,
  runtimeStatus,
  onBack,
  onValidate,
  onSave,
  onApplyMigrations,
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
  const [migrationBusy, setMigrationBusy] = useState(false);
  const [migrationToken, setMigrationToken] = useState("");
  const [migrationResult, setMigrationResult] = useState<SupabaseSchemaApplyResult | null>(null);

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

  const handleApplyMigrations = async () => {
    const projectRef =
      validation?.projectRef
      || runtimeStatus.projectRef
      || deriveProjectRef(form.apiUrl);
    if (!form.apiUrl.trim() || !projectRef || !migrationToken.trim()) {
      setFeedback("Supabase URL, project ref, and a Management API access token are required to apply repo migrations.");
      return;
    }

    setFeedback(null);
    setMigrationBusy(true);
    try {
      const result = await onApplyMigrations({
        apiUrl: form.apiUrl,
        projectRef,
        accessToken: migrationToken,
      });
      setMigrationResult(result);
      setFeedback(`Schema init finished: ${result.appliedCount} applied, ${result.skippedCount} skipped.`);
    } catch (error) {
      setMigrationResult(null);
      setFeedback(error instanceof Error ? error.message : "Unable to apply repo migrations.");
    } finally {
      setMigrationBusy(false);
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
            type="password"
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

      <div className="settings-subpanel">
        <div className="settings-panel-head">
          <div>
            <h3>Schema Initialization</h3>
            <p>
              Apply the checked-in `supabase/migrations` files to the configured project.
              The Management API token is used only for this action and is not saved.
            </p>
          </div>
        </div>

        <div className="settings-grid">
          <label className="settings-field settings-field-full">
            <span>Management API Access Token</span>
            <input
              disabled={busy || migrationBusy}
              onChange={(event) => setMigrationToken(event.target.value)}
              placeholder="Supabase personal access token with database:write scope"
              type="password"
              value={migrationToken}
            />
          </label>
        </div>

        <div className="settings-actions">
          <button
            className="secondary-btn"
            disabled={busy || migrationBusy || !runtimeStatus.runnerReachable}
            onClick={() => void handleApplyMigrations()}
            type="button"
          >
            {migrationBusy ? "Applying..." : "Apply Repo Migrations"}
          </button>
        </div>

        {migrationResult ? (
          <div className="settings-validation">
            {migrationResult.migrations.map((migration) => (
              <div className={`validation-row ${migration.status === "applied" ? "passed" : "warn"}`} key={migration.version}>
                <span>{migration.version} {migration.name}</span>
                <span>{migration.message || migration.status}</span>
              </div>
            ))}
          </div>
        ) : null}
      </div>
    </section>
  );
}
