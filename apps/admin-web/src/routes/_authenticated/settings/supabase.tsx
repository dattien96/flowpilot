import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import {
  SupabaseConfigFields,
  ValidationPanel,
} from "@/routes/setup.supabase";
import {
  invalidateSupabaseRuntimeStatus,
  loadSupabaseRuntimeStatus,
  type SupabaseRuntimeStatus,
} from "@/lib/supabase/runtime-config";

export const Route = createFileRoute("/_authenticated/settings/supabase")({
  component: SupabaseSettingsPage,
});

type ValidationResult = {
  valid: boolean;
  checks: Array<{ key: string; status: "passed" | "failed" | "skipped"; message: string }>;
};

function SupabaseSettingsPage() {
  const [status, setStatus] = useState<SupabaseRuntimeStatus | null>(null);
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
  const [message, setMessage] = useState<string | null>(null);
  const [reloadRequired, setReloadRequired] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let active = true;
    void loadSupabaseRuntimeStatus().then((nextStatus) => {
      if (!active) {
        return;
      }
      setStatus(nextStatus);
      setForm({
        apiUrl: nextStatus.apiUrl ?? "",
        anonKey: nextStatus.anonKey ?? "",
        edgeFunctionUrl: nextStatus.edgeFunctionUrl ?? "",
        serviceRoleKey: "",
      });
    });
    return () => {
      active = false;
    };
  }, []);

  const updateApiUrl = (apiUrl: string) => {
    setForm((current) => ({
      ...current,
      apiUrl,
      edgeFunctionUrl: edgeTouched ? current.edgeFunctionUrl : suggestEdgeUrl(apiUrl),
    }));
    setValidation(null);
  };

  const updateField = (key: "anonKey" | "edgeFunctionUrl" | "serviceRoleKey", value: string) => {
    if (key === "edgeFunctionUrl") {
      setEdgeTouched(true);
    }
    setForm((current) => ({ ...current, [key]: value }));
    setValidation(null);
  };

  const testConnection = async () => {
    setBusy(true);
    setMessage(null);
    try {
      const result = await submit("/api/runtime/supabase-config/validate", form) as ValidationResult;
      setValidation(result);
      setMessage(result.valid ? "Connection checks passed." : "Fix the failed checks before saving.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Connection test failed.");
    } finally {
      setBusy(false);
    }
  };

  const save = async () => {
    setBusy(true);
    setMessage(null);
    try {
      const nextStatus = await submit("/api/runtime/supabase-config", form, "PUT") as SupabaseRuntimeStatus;
      invalidateSupabaseRuntimeStatus();
      setStatus(nextStatus);
      setReloadRequired(true);
      setMessage("Database configuration updated. A page reload is required to apply the changes.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to save Supabase config.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <PageFrame
      title="Supabase Settings"
      description="Manage database connections, API keys, and workspace secrets."
    >
      <SourceNotice status={status} />
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <SupabaseConfigFields
          form={form}
          hasExistingServiceRoleKey={status?.hasServiceRoleKey}
          onApiUrlChange={updateApiUrl}
          onFieldChange={updateField}
          onToggleAnon={() => setShowAnon((value) => !value)}
          onToggleServiceRole={() => setShowServiceRole((value) => !value)}
          showAnon={showAnon}
          showServiceRole={showServiceRole}
        />
        <ValidationPanel message={message} validation={validation} />
        {reloadRequired ? (
          <div className="mt-4 rounded-2xl border border-warning/30 bg-warning/10 px-4 py-3 text-sm">
            Configuration updated successfully. A page reload is required to apply the changes.
            <Button className="ml-3" onClick={() => window.location.reload()} type="button">
              Reload Now
            </Button>
          </div>
        ) : null}
        <div className="mt-6 flex flex-wrap gap-3">
          <Button disabled={busy || status?.runnerReachable === false} onClick={testConnection} variant="secondary">
            Test Connection
          </Button>
          <Button disabled={busy || status?.runnerReachable === false} onClick={save}>
            Save Changes
          </Button>
        </div>
      </section>
    </PageFrame>
  );
}

function SourceNotice({ status }: { status: SupabaseRuntimeStatus | null }) {
  if (!status) {
    return <div className="rounded-2xl border border-border bg-card/80 px-4 py-3 text-sm">Loading runtime status...</div>;
  }
  const message =
    status.mode === "config"
      ? "Workspace config is active. Environment variables are ignored until saved config is reset."
      : status.mode === "env"
        ? "FlowPilot is using local environment variables because no saved workspace config exists. Saving this form creates workspace config and makes it active after reload."
        : "No saved config or complete env fallback is available. Demo mode is active.";
  return (
    <div className="rounded-2xl border border-border bg-card/80 px-4 py-3 text-sm">
      <span className="font-semibold uppercase">{status.mode}</span>
      <span className="ml-2 text-muted-foreground">{message}</span>
      {!status.runnerReachable ? (
        <p className="mt-2 text-danger">Local runner is offline, so saved config cannot be read or written. {status.lastError}</p>
      ) : null}
      <p className="mt-2 text-xs text-muted-foreground">
        Edge functions: {status.edgeFunctionsReady ? "ready" : "not configured"} | Service role: {status.hasServiceRoleKey ? "stored" : "missing"}
      </p>
    </div>
  );
}

async function submit(path: string, form: Record<string, string>, method = "POST") {
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
