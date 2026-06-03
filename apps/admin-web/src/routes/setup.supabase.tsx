import { createFileRoute, Link } from "@tanstack/react-router";
import { Eye, EyeOff, ShieldAlert } from "lucide-react";
import type { ReactNode } from "react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { resetBrowserGatewayBundle } from "@/data/repository/browser-factory";
import { resetBrowserSupabaseClient } from "@/data/supabase/client";
import type { SupabaseRuntimeStatus } from "@/lib/supabase/runtime-config";

export const Route = createFileRoute("/setup/supabase")({
  component: SetupSupabasePage,
});

type ValidationCheck = {
  key: string;
  status: "passed" | "failed" | "skipped";
  message: string;
};

type ValidationResult = {
  valid: boolean;
  checks: ValidationCheck[];
  projectRef?: string;
};

export function SetupSupabasePage() {
  const [form, setForm] = useState({
    apiUrl: "",
    anonKey: "",
    edgeFunctionUrl: "",
    serviceRoleKey: "",
  });
  const [edgeTouched, setEdgeTouched] = useState(false);
  const [showAnon, setShowAnon] = useState(false);
  const [showServiceRole, setShowServiceRole] = useState(false);
  const [validation, setValidation] = useState<ValidationResult | null>(null);
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const updateApiUrl = (apiUrl: string) => {
    setForm((current) => ({
      ...current,
      apiUrl,
      edgeFunctionUrl: edgeTouched ? current.edgeFunctionUrl : suggestEdgeUrl(apiUrl),
    }));
    setValidation(null);
  };

  const updateField = (key: keyof typeof form, value: string) => {
    if (key === "edgeFunctionUrl") {
      setEdgeTouched(true);
    }
    setForm((current) => ({ ...current, [key]: value }));
    setValidation(null);
  };

  const testConnection = async () => {
    setBusy(true);
    setStatusMessage(null);
    try {
      const result = await submitSupabaseConfig("/api/runtime/supabase-config/validate", form) as ValidationResult;
      setValidation(result);
      setStatusMessage(result.valid ? "Connection checks passed." : "Fix the failed checks before saving.");
    } catch (error) {
      setStatusMessage(error instanceof Error ? error.message : "Connection test failed.");
    } finally {
      setBusy(false);
    }
  };

  const saveConfig = async () => {
    setBusy(true);
    setStatusMessage(null);
    try {
      await submitSupabaseConfig("/api/runtime/supabase-config", form, "PUT");
      resetBrowserGatewayBundle();
      resetBrowserSupabaseClient();
      window.location.assign("/login");
    } catch (error) {
      setStatusMessage(error instanceof Error ? error.message : "Unable to save Supabase config.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="noise-bg flex min-h-screen items-center justify-center px-4 py-10">
      <div className="panel-shadow w-full max-w-2xl rounded-[2rem] border border-border/80 bg-card/95 p-8 backdrop-blur">
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          FlowPilot
        </p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight">
          Configure Supabase Workspace
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          Configure a dedicated Supabase connection to store and persist workspace data.
        </p>

        <SupabaseConfigFields
          form={form}
          showAnon={showAnon}
          showServiceRole={showServiceRole}
          onApiUrlChange={updateApiUrl}
          onFieldChange={updateField}
          onToggleAnon={() => setShowAnon((value) => !value)}
          onToggleServiceRole={() => setShowServiceRole((value) => !value)}
        />

        <ValidationPanel validation={validation} message={statusMessage} />

        <div className="mt-6 flex flex-col gap-3 sm:flex-row">
          <Button disabled={busy} onClick={testConnection} variant="secondary">
            {busy ? "Testing..." : "Test Connection"}
          </Button>
          <Button disabled={busy || validation?.valid !== true} onClick={saveConfig}>
            Save & Configure
          </Button>
          <Link className="inline-flex items-center justify-center rounded-full px-4 py-2 text-sm font-semibold text-muted-foreground hover:bg-muted/80" to="/login">
            Return to Login
          </Link>
        </div>
      </div>
    </div>
  );
}

export function SupabaseConfigFields({
  form,
  hasExistingServiceRoleKey,
  onApiUrlChange,
  onFieldChange,
  onToggleAnon,
  onToggleServiceRole,
  showAnon,
  showServiceRole,
}: {
  form: { apiUrl: string; anonKey: string; edgeFunctionUrl: string; serviceRoleKey: string };
  hasExistingServiceRoleKey?: boolean;
  onApiUrlChange: (value: string) => void;
  onFieldChange: (key: "anonKey" | "edgeFunctionUrl" | "serviceRoleKey", value: string) => void;
  onToggleAnon: () => void;
  onToggleServiceRole: () => void;
  showAnon: boolean;
  showServiceRole: boolean;
}) {
  return (
    <div className="mt-8 grid gap-4">
      <Field label="Supabase URL">
        <input
          className="w-full rounded-2xl border border-border bg-background px-4 py-3 outline-none transition-all focus:border-transparent focus:ring-2 focus:ring-accent"
          onChange={(event) => onApiUrlChange(event.target.value)}
          placeholder="https://project-ref.supabase.co"
          value={form.apiUrl}
        />
      </Field>
      <SecretField
        label="Anon Key"
        onChange={(value) => onFieldChange("anonKey", value)}
        onToggle={onToggleAnon}
        placeholder="Public anon key"
        reveal={showAnon}
        value={form.anonKey}
      />
      <Field label="Edge Function URL">
        <input
          className="w-full rounded-2xl border border-border bg-background px-4 py-3 outline-none transition-all focus:border-transparent focus:ring-2 focus:ring-accent"
          onChange={(event) => onFieldChange("edgeFunctionUrl", event.target.value)}
          placeholder="https://project-ref.supabase.co/functions/v1"
          value={form.edgeFunctionUrl}
        />
      </Field>
      <SecretField
        label="Service Role Key"
        note={hasExistingServiceRoleKey ? "Leave empty to keep existing key, or enter a new key to overwrite." : undefined}
        onChange={(value) => onFieldChange("serviceRoleKey", value)}
        onToggle={onToggleServiceRole}
        placeholder={hasExistingServiceRoleKey ? "****************" : "Privileged service role key"}
        reveal={showServiceRole}
        value={form.serviceRoleKey}
        warning
      />
    </div>
  );
}

function Field({ children, label }: { children: ReactNode; label: string }) {
  return (
    <label className="block space-y-2">
      <span className="text-sm text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

function SecretField({
  label,
  note,
  onChange,
  onToggle,
  placeholder,
  reveal,
  value,
  warning,
}: {
  label: string;
  note?: string;
  onChange: (value: string) => void;
  onToggle: () => void;
  placeholder: string;
  reveal: boolean;
  value: string;
  warning?: boolean;
}) {
  return (
    <Field label={label}>
      <div className="flex overflow-hidden rounded-2xl border border-border bg-background focus-within:ring-2 focus-within:ring-accent">
        <input
          className="min-w-0 flex-1 bg-transparent px-4 py-3 outline-none"
          onChange={(event) => onChange(event.target.value)}
          placeholder={placeholder}
          type={reveal ? "text" : "password"}
          value={value}
        />
        <button className="px-4 text-muted-foreground hover:text-foreground" onClick={onToggle} type="button">
          {reveal ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </button>
      </div>
      {warning ? (
        <p className="inline-flex items-center gap-2 text-xs text-warning">
          <ShieldAlert className="h-3.5 w-3.5" />
          Privileged Secret
        </p>
      ) : null}
      {note ? <p className="text-xs text-muted-foreground">{note}</p> : null}
    </Field>
  );
}

export function ValidationPanel({
  message,
  validation,
}: {
  message: string | null;
  validation: ValidationResult | null;
}) {
  if (!message && !validation) {
    return null;
  }
  return (
    <div className="mt-6 rounded-2xl border border-border bg-background/70 p-4">
      {message ? <p className="text-sm font-medium">{message}</p> : null}
      {validation ? (
        <div className="mt-3 grid gap-2">
          {validation.checks.map((item) => (
            <p
              className={`text-sm ${
                item.status === "passed"
                  ? "text-success"
                  : item.status === "failed"
                    ? "text-danger"
                    : "text-muted-foreground"
              }`}
              key={item.key}
            >
              {item.message}
            </p>
          ))}
        </div>
      ) : null}
    </div>
  );
}

async function submitSupabaseConfig(path: string, form: Record<string, string>, method = "POST") {
  const response = await fetch(path, {
    method,
    headers: { "content-type": "application/json" },
    body: JSON.stringify(form),
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error ?? "Supabase config request failed.");
  }
  return payload as ValidationResult | SupabaseRuntimeStatus;
}

function suggestEdgeUrl(apiUrl: string) {
  const trimmed = apiUrl.trim().replace(/\/+$/, "");
  return trimmed ? `${trimmed}/functions/v1` : "";
}
