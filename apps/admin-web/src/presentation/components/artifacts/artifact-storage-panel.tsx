"use client";

import { useState } from "react";
import { CheckCircle2, RefreshCw, Save } from "lucide-react";

import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/presentation/components/ui/button";
import type { LocalRunnerStorageDriver } from "@/domain/model/entity/local-runner";

interface ArtifactStoragePanelProps {
  storageDriver: LocalRunnerStorageDriver;
}

export function ArtifactStoragePanel({ storageDriver }: ArtifactStoragePanelProps) {
  const [driverKey, setDriverKey] = useState(storageDriver.driverKey);
  const [enabled, setEnabled] = useState(storageDriver.enabled);
  const [remoteRootPath, setRemoteRootPath] = useState(storageDriver.remoteRootPath);
  const [remoteFolderName, setRemoteFolderName] = useState(storageDriver.remoteFolderName);
  const [saving, setSaving] = useState(false);
  const [validating, setValidating] = useState(false);
  const [message, setMessage] = useState<string | null>(storageDriver.lastError || null);

  async function saveDriver(validateAfterSave: boolean) {
    setSaving(true);
    setMessage(null);

    try {
      const response = await fetch("/api/local-runner/storage-driver", {
        method: "PUT",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({
          driverKey,
          enabled,
          remoteRootPath,
          remoteFolderName,
        }),
      });

      const data = (await response.json()) as LocalRunnerStorageDriver | { error?: string };

      if (!response.ok) {
        throw new Error("error" in data && data.error ? data.error : "Unable to save storage driver.");
      }

      const nextDriver = data as LocalRunnerStorageDriver;
      setDriverKey(nextDriver.driverKey);
      setEnabled(nextDriver.enabled);
      setRemoteRootPath(nextDriver.remoteRootPath);
      setRemoteFolderName(nextDriver.remoteFolderName);
      setMessage("Storage driver saved.");

      if (validateAfterSave) {
        setValidating(true);
        const validateResponse = await fetch("/api/local-runner/storage-driver", {
          method: "POST",
        });
        const validateData = (await validateResponse.json()) as
          | LocalRunnerStorageDriver
          | { error?: string };

        if (!validateResponse.ok) {
          throw new Error(
            "error" in validateData && validateData.error
              ? validateData.error
              : "Unable to validate storage driver.",
          );
        }

        const validatedDriver = validateData as LocalRunnerStorageDriver;
        setMessage(
          validatedDriver.lastError ? validatedDriver.lastError : "Storage driver validated.",
        );
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to update storage driver.");
    } finally {
      setSaving(false);
      setValidating(false);
    }
  }

  return (
    <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Artifact Storage
          </p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight">
            Connect the driver folder
          </h2>
          <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
            Configure the local driver root that receives synced artifacts and backups.
          </p>
        </div>
        <Badge tone={enabled ? "success" : "warning"}>{enabled ? "enabled" : "disabled"}</Badge>
      </div>

      <div className="mt-6 grid gap-4 xl:grid-cols-2">
        <label className="space-y-2">
          <span className="text-sm font-medium">Driver key</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm outline-none"
            value={driverKey}
            onChange={(event) => setDriverKey(event.target.value)}
          />
        </label>

        <label className="space-y-2">
          <span className="text-sm font-medium">Folder name</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm outline-none"
            value={remoteFolderName}
            onChange={(event) => setRemoteFolderName(event.target.value)}
          />
        </label>

        <label className="space-y-2 xl:col-span-2">
          <span className="text-sm font-medium">Remote root path</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm outline-none"
            placeholder="D:\\FlowPilot-Driver or /Users/me/FlowPilot-Driver"
            value={remoteRootPath}
            onChange={(event) => setRemoteRootPath(event.target.value)}
          />
        </label>
      </div>

      <div className="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <InfoTile label="Last validated" value={storageDriver.lastValidatedAt || "Never"} />
        <InfoTile label="Last synced" value={storageDriver.lastSyncedAt || "Never"} />
        <InfoTile label="Last error" value={storageDriver.lastError || "None"} />
        <InfoTile label="Updated at" value={storageDriver.updatedAt || "Never"} />
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-3">
        <button
          type="button"
          className="inline-flex items-center gap-2 rounded-full border border-border px-4 py-2 text-sm font-semibold text-foreground transition-colors hover:bg-muted"
          onClick={() => setEnabled((current) => !current)}
        >
          <CheckCircle2 className="size-4" />
          {enabled ? "Disable" : "Enable"}
        </button>
        <Button
          type="button"
          disabled={saving}
          onClick={() => {
            void saveDriver(false);
          }}
        >
          <Save className="mr-2 size-4" />
          {saving ? "Saving..." : "Save driver"}
        </Button>
        <Button
          type="button"
          variant="secondary"
          disabled={saving || validating}
          onClick={() => {
            void saveDriver(true);
          }}
        >
          <RefreshCw className="mr-2 size-4" />
          {validating ? "Validating..." : "Save and test"}
        </Button>
        <p className="text-sm text-muted-foreground">
          {message ?? "The runner persists this config locally and uses it for artifact sync."}
        </p>
      </div>
    </section>
  );
}

function InfoTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-border bg-card/80 px-4 py-3">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-2 break-words text-sm font-medium">{value}</p>
    </div>
  );
}
