"use client";

import type { ReactNode } from "react";
import { useState } from "react";
import { Download, ExternalLink, Search, UploadCloud } from "lucide-react";
import { useRouter } from "next/navigation";

import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/presentation/components/ui/button";
import type { LocalRunnerArtifact, LocalRunnerStorageDriver } from "@/domain/model/entity/local-runner";

interface ArtifactBrowserPanelProps {
  artifacts: LocalRunnerArtifact[];
  storageDriver: LocalRunnerStorageDriver;
}

export function ArtifactBrowserPanel({
  artifacts,
  storageDriver,
}: ArtifactBrowserPanelProps) {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [sourceKind, setSourceKind] = useState("all");
  const [projectId, setProjectId] = useState("all");
  const [featureId, setFeatureId] = useState("all");
  const [runId, setRunId] = useState("all");
  const [selectedArtifactId, setSelectedArtifactId] = useState(artifacts[0]?.artifactId ?? "");
  const [backupScope, setBackupScope] = useState<"all" | "run">("all");
  const [backupMessage, setBackupMessage] = useState<string | null>(null);
  const [syncingArtifactId, setSyncingArtifactId] = useState<string | null>(null);
  const [backuping, setBackuping] = useState(false);

  const filteredArtifacts = artifacts.filter((artifact) => {
    const matchesQuery =
      query.trim() === "" ||
      [
        artifact.title,
        artifact.contentMarkdown,
        artifact.localPath,
        artifact.workflowRunId,
        artifact.workflowStepKey,
      ]
        .join(" ")
        .toLowerCase()
        .includes(query.toLowerCase());
    const matchesSource = sourceKind === "all" || artifact.sourceKind === sourceKind;
    const matchesProject = projectId === "all" || artifact.projectId === projectId;
    const matchesFeature = featureId === "all" || artifact.featureId === featureId;
    const matchesRun = runId === "all" || artifact.workflowRunId === runId;

    return matchesQuery && matchesSource && matchesProject && matchesFeature && matchesRun;
  });

  const selectedArtifact =
    filteredArtifacts.find((artifact) => artifact.artifactId === selectedArtifactId) ??
    filteredArtifacts[0] ??
    null;

  async function syncArtifact(artifactId: string) {
    setSyncingArtifactId(artifactId);
    try {
      const response = await fetch(`/api/local-runner/artifacts/${artifactId}/sync`, {
        method: "POST",
      });
      if (!response.ok) {
        const payload = (await response.json()) as { error?: string };
        throw new Error(payload.error ?? "Unable to sync artifact.");
      }
      router.refresh();
    } catch (error) {
      setBackupMessage(error instanceof Error ? error.message : "Unable to sync artifact.");
    } finally {
      setSyncingArtifactId(null);
    }
  }

  async function createBackup(scope: "all" | "run") {
    setBackuping(true);
    setBackupMessage(null);
    try {
      const response = await fetch("/api/local-runner/backup", {
        method: "POST",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({
          scope,
          runId: scope === "run" ? selectedArtifact?.workflowRunId ?? null : null,
        }),
      });
      const payload = (await response.json()) as { backupPath?: string; error?: string };
      if (!response.ok) {
        throw new Error(payload.error ?? "Unable to create backup.");
      }
      setBackupMessage(payload.backupPath ? `Backup created: ${payload.backupPath}` : "Backup created.");
    } catch (error) {
      setBackupMessage(error instanceof Error ? error.message : "Unable to create backup.");
    } finally {
      setBackuping(false);
    }
  }

  const projectOptions = uniqueValues(artifacts.map((artifact) => artifact.projectId));
  const featureOptions = uniqueValues(artifacts.map((artifact) => artifact.featureId));
  const runOptions = uniqueValues(artifacts.map((artifact) => artifact.workflowRunId));
  const sourceOptions = uniqueValues(artifacts.map((artifact) => artifact.sourceKind));

  return (
    <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Artifacts
          </p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight">
            Generated files by flow run
          </h2>
          <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
            Browse generated markdown, preview content, sync file-backed artifacts, and export
            backups to the configured driver folder.
          </p>
        </div>
        <Badge tone={storageDriver.enabled ? "success" : "warning"}>
          {storageDriver.enabled ? "driver ready" : "driver paused"}
        </Badge>
      </div>

      <div className="mt-6 grid gap-3 xl:grid-cols-3">
        <label className="space-y-2 xl:col-span-3">
          <span className="text-sm font-medium">Search</span>
          <div className="flex items-center gap-3 rounded-2xl border border-border bg-card px-4 py-3">
            <Search className="size-4 text-muted-foreground" />
            <input
              className="w-full bg-transparent text-sm outline-none"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search title, preview, run id, or path"
            />
          </div>
        </label>

        <FilterSelect label="Source" value={sourceKind} onChange={setSourceKind}>
          <option value="all">All sources</option>
          {sourceOptions.map((item) => (
            <option key={item} value={item}>
              {item}
            </option>
          ))}
        </FilterSelect>

        <FilterSelect label="Project" value={projectId} onChange={setProjectId}>
          <option value="all">All projects</option>
          {projectOptions.map((item) => (
            <option key={item} value={item}>
              {item}
            </option>
          ))}
        </FilterSelect>

        <FilterSelect label="Feature" value={featureId} onChange={setFeatureId}>
          <option value="all">All features</option>
          {featureOptions.map((item) => (
            <option key={item} value={item}>
              {item}
            </option>
          ))}
        </FilterSelect>

        <FilterSelect label="Run" value={runId} onChange={setRunId}>
          <option value="all">All runs</option>
          {runOptions.map((item) => (
            <option key={item} value={item}>
              {item}
            </option>
          ))}
        </FilterSelect>
      </div>

      <div className="mt-6 flex flex-wrap items-center gap-3">
        <label className="space-y-1">
          <span className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
            Backup scope
          </span>
          <select
            className="rounded-2xl border border-border bg-card px-4 py-2 text-sm outline-none"
            value={backupScope}
            onChange={(event) => setBackupScope(event.target.value as "all" | "run")}
          >
            <option value="all">All artifacts</option>
            <option value="run">Selected run</option>
          </select>
        </label>
        <Button
          type="button"
          variant="secondary"
          disabled={backuping}
          onClick={() => {
            void createBackup("all");
          }}
        >
          <Download className="mr-2 size-4" />
          {backuping ? "Backing up..." : "Backup all"}
        </Button>
        <Button
          type="button"
          variant="secondary"
          disabled={backuping || !selectedArtifact}
          onClick={() => {
            void createBackup(backupScope);
          }}
        >
          <Download className="mr-2 size-4" />
          {backupScope === "run" ? "Backup selected run" : "Backup all artifacts"}
        </Button>
        {backupMessage ? <p className="text-sm text-muted-foreground">{backupMessage}</p> : null}
      </div>

      <div className="mt-6 grid gap-4 xl:grid-cols-[1fr_1.1fr]">
        <div className="space-y-3">
          {filteredArtifacts.length === 0 ? (
            <p className="text-sm text-muted-foreground">No artifacts match the current filters.</p>
          ) : (
            filteredArtifacts.map((artifact) => {
              const selected = artifact.artifactId === selectedArtifact?.artifactId;
              const syncable = Boolean(artifact.localPath);
              return (
                <button
                  key={artifact.artifactId}
                  type="button"
                  onClick={() => setSelectedArtifactId(artifact.artifactId)}
                  className={`w-full rounded-[1.25rem] border p-4 text-left transition-colors ${
                    selected
                      ? "border-accent bg-accent/8"
                      : "border-border bg-card/80 hover:bg-muted/60"
                  }`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="text-base font-semibold">{artifact.title}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {artifact.projectId} / {artifact.featureId} / {artifact.workflowRunId}
                      </p>
                    </div>
                    <Badge tone={artifact.syncStatus === "synced" ? "success" : "warning"}>
                      {artifact.sourceKind}
                    </Badge>
                  </div>
                  <p className="mt-3 line-clamp-2 text-sm text-muted-foreground">
                    {artifact.previewMarkdown || artifact.contentMarkdown || "No preview available."}
                  </p>
                  <div className="mt-4 flex flex-wrap items-center gap-2">
                    <Badge tone="neutral">{artifact.syncStatus}</Badge>
                    <Badge tone="neutral">{artifact.workflowStepKey}</Badge>
                    <Badge tone="neutral">
                      {artifact.createdAt.slice(0, 19).replace("T", " ")}
                    </Badge>
                    <span className="text-xs text-muted-foreground">
                      {syncable ? "file-backed" : "demo record"}
                    </span>
                  </div>
                </button>
              );
            })
          )}
        </div>

        <div className="rounded-[1.25rem] border border-border bg-card/80 p-5">
          {selectedArtifact ? (
            <div className="space-y-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="text-xl font-semibold">{selectedArtifact.title}</h3>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {selectedArtifact.projectId} / {selectedArtifact.featureId} /{" "}
                    {selectedArtifact.workflowRunId}
                  </p>
                </div>
                <Badge tone={selectedArtifact.syncStatus === "synced" ? "success" : "warning"}>
                  {selectedArtifact.syncStatus}
                </Badge>
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <InfoTile label="Local path" value={selectedArtifact.localPath || "Not file-backed"} />
                <InfoTile label="Remote link" value={selectedArtifact.remoteUrl || "Not synced"} />
                <InfoTile label="Provider" value={selectedArtifact.providerKey || "Unknown"} />
                <InfoTile label="Step" value={selectedArtifact.workflowStepKey} />
              </div>

              <div className="flex flex-wrap items-center gap-3">
                <Button
                  type="button"
                  disabled={
                    syncingArtifactId === selectedArtifact.artifactId ||
                    !selectedArtifact.localPath ||
                    !storageDriver.enabled
                  }
                  onClick={() => {
                    void syncArtifact(selectedArtifact.artifactId);
                  }}
                >
                  <UploadCloud className="mr-2 size-4" />
                  {syncingArtifactId === selectedArtifact.artifactId ? "Syncing..." : "Sync"}
                </Button>
                {selectedArtifact.remoteUrl ? (
                  <a
                    className="inline-flex items-center gap-2 rounded-full border border-border px-4 py-2 text-sm font-semibold hover:bg-muted"
                    href={selectedArtifact.remoteUrl}
                    target="_blank"
                    rel="noreferrer"
                  >
                    <ExternalLink className="size-4" />
                    Open remote
                  </a>
                ) : null}
                {selectedArtifact.localPath ? (
                  <span className="text-sm text-muted-foreground">{selectedArtifact.localPath}</span>
                ) : null}
              </div>

              <pre className="max-h-[32rem] overflow-auto whitespace-pre-wrap rounded-2xl border border-border bg-background px-4 py-4 text-sm">
                {selectedArtifact.contentMarkdown || "No markdown content available."}
              </pre>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">Select an artifact to inspect its content.</p>
          )}
        </div>
      </div>
    </section>
  );
}

function FilterSelect({
  label,
  value,
  onChange,
  children,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  children: ReactNode;
}) {
  return (
    <label className="space-y-2">
      <span className="text-sm font-medium">{label}</span>
      <select
        className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm outline-none"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {children}
      </select>
    </label>
  );
}

function InfoTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-border bg-background p-4">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-2 break-words text-sm font-medium">{value}</p>
    </div>
  );
}

function uniqueValues(values: string[]) {
  return Array.from(new Set(values.filter((value) => value.trim() !== "")));
}
