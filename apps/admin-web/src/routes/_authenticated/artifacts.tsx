import { useCallback, useEffect, useRef, useState } from "react";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";
import type { Project } from "@/domain/model/entity/project";
import type {
  ArtifactDefinition,
  ArtifactRun,
  WorkflowRun,
} from "@/domain/model/entity/workflow-engine";
import { ListProjectsUseCase } from "@/domain/usecase/projects/list-projects-usecase";
import { ListArtifactDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-artifact-definitions-usecase";
import { ListArtifactRunsUseCase } from "@/domain/usecase/workflow-engine/list-artifact-runs-usecase";
import { ListWorkflowRunsUseCase } from "@/domain/usecase/workflow-engine/list-workflow-runs-usecase";
import { SaveArtifactDefinitionUseCase } from "@/domain/usecase/workflow-engine/save-artifact-definition-usecase";
import { ArtifactRunBrowserPanel } from "@/presentation/components/artifacts/artifact-run-browser-panel";
import { ArtifactCloudStoragePanel } from "@/presentation/components/artifacts/artifact-cloud-storage-panel";

export const Route = createFileRoute("/_authenticated/artifacts")({
  component: ArtifactsPage,
});

export function ArtifactsPage() {
  const location = useLocation();
  const gatewayBundle = useRef(createGatewayBundle());
  const listArtifactDefinitionsUseCase = useRef(
    new ListArtifactDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway),
  );
  const listArtifactRunsUseCase = useRef(
    new ListArtifactRunsUseCase(gatewayBundle.current.workflowEngineGateway),
  );
  const listWorkflowRunsUseCase = useRef(
    new ListWorkflowRunsUseCase(gatewayBundle.current.workflowEngineGateway),
  );
  const saveArtifactDefinitionUseCase = useRef(
    new SaveArtifactDefinitionUseCase(gatewayBundle.current.workflowEngineGateway),
  );
  const listProjectsUseCase = useRef(new ListProjectsUseCase(gatewayBundle.current.projectGateway));

  const [loading, setLoading] = useState(true);
  const [savingKey, setSavingKey] = useState<string | null>(null);
  const [artifactDefinitions, setArtifactDefinitions] = useState<ArtifactDefinition[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [localArtifacts, setLocalArtifacts] = useState<LocalRunnerArtifact[]>([]);
  const [artifactRuns, setArtifactRuns] = useState<ArtifactRun[]>([]);
  const [activeTab, setActiveTab] = useState<"generated" | "storage" | "catalog">("generated");

  const loadArtifactsPage = useCallback(async () => {
    setLoading(true);
    try {
      const [
        definitions,
        loadedProjects,
        loadedLocalArtifacts,
        loadedArtifactRuns,
        loadedWorkflowRuns,
      ] = await Promise.all([
        listArtifactDefinitionsUseCase.current.execute(),
        listProjectsUseCase.current.execute(),
        gatewayBundle.current.localRunnerGateway.listArtifacts(),
        listArtifactRunsUseCase.current.execute(),
        listWorkflowRunsUseCase.current.execute(),
      ]);
      const validWorkflowRunIds = new Set(loadedWorkflowRuns.map((run: WorkflowRun) => run.id));
      setArtifactDefinitions(definitions);
      setProjects(loadedProjects);
      setLocalArtifacts(
        loadedLocalArtifacts.filter(
          (artifact: LocalRunnerArtifact) =>
            artifact.projectId.trim() !== "" &&
            artifact.workflowRunId.trim() !== "" &&
            validWorkflowRunIds.has(artifact.workflowRunId),
        ),
      );
      setArtifactRuns(loadedArtifactRuns);
    } catch {
      setArtifactDefinitions([]);
      setProjects([]);
      setLocalArtifacts([]);
      setArtifactRuns([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadArtifactsPage();
  }, [loadArtifactsPage]);

  const updateDefinition = (key: string, patch: Partial<ArtifactDefinition>) => {
    setArtifactDefinitions((current) =>
      current.map((definition) =>
        definition.key === key ? { ...definition, ...patch } : definition,
      ),
    );
  };

  const saveDefinition = async (definition: ArtifactDefinition) => {
    if (!definition.key.trim() || !definition.name.trim()) {
      window.alert("Artifact key and name are required.");
      return;
    }

    setSavingKey(definition.key);
    try {
      const saved = await saveArtifactDefinitionUseCase.current.execute(definition);
      setArtifactDefinitions((current) => {
        const next = current.filter((item) => item.key !== saved.key);
        next.unshift(saved);
        return next;
      });
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to save artifact definition.");
    } finally {
      setSavingKey(null);
    }
  };

  if (location.pathname !== "/artifacts") {
    return <Outlet />;
  }

  return (
    <PageFrame
      title="Artifacts"
      description="Manage the global artifact catalog and inspect sync state across all projects."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/settings/runner">
            <Button variant="secondary">Runner</Button>
          </Link>
          <Link to="/settings/artifacts">
            <Button variant="secondary">Artifact settings</Button>
          </Link>
          <Link to="/settings/mcp-servers">
            <Button variant="secondary">MCP Servers</Button>
          </Link>
        </div>
      }
    >
      <div className="space-y-6">
        {/* Modern Tab switcher with glassmorphic pill buttons */}
        <div className="flex flex-wrap border border-border/30 bg-[#0f1119]/70 backdrop-blur-md rounded-2xl p-1 gap-1.5 w-fit shrink-0">
          <button
            type="button"
            className={`px-5 py-2.5 text-sm font-semibold tracking-wide transition-all duration-200 rounded-xl cursor-pointer ${
              activeTab === "generated"
                ? "text-primary bg-background shadow-md border border-border/20"
                : "text-muted-foreground hover:text-foreground hover:bg-muted/10"
            }`}
            onClick={() => setActiveTab("generated")}
          >
            Artifact generated list
          </button>
          <button
            type="button"
            className={`px-5 py-2.5 text-sm font-semibold tracking-wide transition-all duration-200 rounded-xl cursor-pointer ${
              activeTab === "storage"
                ? "text-primary bg-background shadow-md border border-border/20"
                : "text-muted-foreground hover:text-foreground hover:bg-muted/10"
            }`}
            onClick={() => setActiveTab("storage")}
          >
            Shared Cloud Storage Setting
          </button>
          <button
            type="button"
            className={`px-5 py-2.5 text-sm font-semibold tracking-wide transition-all duration-200 rounded-xl cursor-pointer ${
              activeTab === "catalog"
                ? "text-primary bg-background shadow-md border border-border/20"
                : "text-muted-foreground hover:text-foreground hover:bg-muted/10"
            }`}
            onClick={() => setActiveTab("catalog")}
          >
            Catalog definition
          </button>
        </div>

        {/* Tab contents */}
        {activeTab === "generated" && (
          loading ? (
            <section className="rounded-[1.6rem] border border-dashed border-border/60 bg-card/45 p-6 text-sm text-muted-foreground">
              Loading artifacts...
            </section>
          ) : (
            <ArtifactRunBrowserPanel
              artifactRuns={artifactRuns}
              localArtifacts={localArtifacts}
              loadRemoteArtifactContent={async (artifactRun) => {
                if (!artifactRun.id.trim() || !artifactRun.remotePath.trim()) {
                  return null;
                }

                const params = new URLSearchParams({
                  remotePath: artifactRun.remotePath,
                  storageProvider: artifactRun.storageProvider ?? "supabase",
                });
                if (artifactRun.remoteObjectId?.trim()) {
                  params.set("remoteObjectId", artifactRun.remoteObjectId.trim());
                }
                if (artifactRun.projectId?.trim()) {
                  params.set("projectId", artifactRun.projectId.trim());
                }

                const response = await fetch(
                  `/api/local-runner/artifacts/${artifactRun.id}/open?${params.toString()}`,
                );
                if (!response.ok) {
                  return null;
                }

                return await response.text();
              }}
              onArtifactsChanged={loadArtifactsPage}
              projects={projects}
              scopeLabel="All projects"
              showProjectFilter
            />
          )
        )}

        {activeTab === "storage" && (
          projects.length > 0 ? (
            <ArtifactCloudStoragePanel projects={projects} />
          ) : (
            <section className="rounded-[1.6rem] border border-dashed border-border bg-card/45 p-6 text-sm text-muted-foreground">
              No projects found to configure storage.
            </section>
          )
        )}

        {activeTab === "catalog" && (
          <section className="rounded-[1.6rem] border border-border/60 bg-card/45 p-6 shadow-sm backdrop-blur-sm">
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="font-mono text-xs font-semibold uppercase tracking-[0.28em] text-primary/80">
                  Artifact Catalog
                </p>
                <h2 className="mt-3 text-2xl font-bold tracking-tight">Definition catalog</h2>
                <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                  Step definitions bind to these artifact keys. Each record defines the local and
                  remote path template used by workflow runs.
                </p>
              </div>
              <Link to="/artifacts/create">
                <Button
                  disabled={loading}
                  className="rounded-xl shadow-sm transition-all duration-300 hover:opacity-90 hover:shadow-md cursor-pointer"
                >
                  Create Artifact
                </Button>
              </Link>
            </div>

            <div className="mt-6 grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
              {loading ? (
                <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-5 text-sm text-muted-foreground md:col-span-2 lg:col-span-3">
                  Loading...
                </div>
              ) : null}
              {artifactDefinitions.map((definition) => (
                <details
                  key={definition.key}
                  className="group relative flex flex-col justify-between overflow-hidden rounded-[1.8rem] border border-border/60 bg-gradient-to-b from-card/90 to-background/40 p-6 shadow-sm backdrop-blur-md transition-all duration-300 hover:border-primary/20 hover:shadow-xl [&[open]]:border-primary/30 [&[open]]:shadow-lg"
                >
                  <div className="absolute right-0 top-0 h-[3px] left-0 bg-gradient-to-r from-violet-500/85 via-purple-500/85 to-blue-500/85 opacity-50 transition-opacity group-hover:opacity-100 group-[[open]]:opacity-100" />

                  <summary className="flex cursor-pointer list-none items-start justify-between gap-4 outline-none">
                    <div className="min-w-0 space-y-1">
                      <p className="truncate text-lg font-bold tracking-tight text-foreground transition-colors duration-300 group-hover:text-primary">
                        {definition.name}
                      </p>
                      <p className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
                        Key: {definition.key}
                      </p>
                      <p className="mt-2 text-xs font-medium text-muted-foreground/80">
                        Default file:{" "}
                        <span className="rounded border border-border/30 bg-muted/60 px-1.5 py-0.5 font-mono text-[11px]">
                          {definition.defaultFileName || "None"}
                        </span>
                      </p>
                    </div>
                    <span className="shrink-0 rounded border border-border bg-muted/30 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/80 transition-all duration-300 group-hover:border-primary/20 group-hover:bg-primary group-hover:text-primary-foreground">
                      Edit
                    </span>
                  </summary>

                  <div className="mt-6 grid gap-4 border-t border-border/40 pt-4">
                    <label className="flex flex-col space-y-1.5 text-xs">
                      <span className="mb-0.5 font-semibold text-muted-foreground">Name</span>
                      <input
                        className="w-full rounded-xl border border-border/80 bg-background/50 px-3.5 py-2 text-sm font-medium text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-primary/40"
                        value={definition.name}
                        onChange={(event) =>
                          updateDefinition(definition.key, { name: event.target.value })
                        }
                      />
                    </label>
                    <label className="flex flex-col space-y-1.5 text-xs">
                      <span className="mb-0.5 font-semibold text-muted-foreground">Default file name</span>
                      <input
                        className="w-full rounded-xl border border-border/80 bg-background/50 px-3.5 py-2 text-sm font-medium text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-primary/40"
                        value={definition.defaultFileName}
                        onChange={(event) =>
                          updateDefinition(definition.key, { defaultFileName: event.target.value })
                        }
                      />
                    </label>
                    <label className="flex flex-col space-y-1.5 text-xs">
                      <span className="mb-0.5 font-semibold text-muted-foreground">Description</span>
                      <textarea
                        className="min-h-20 w-full resize-y rounded-xl border border-border/80 bg-background/50 px-3.5 py-2 text-sm font-medium text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-primary/40"
                        value={definition.description}
                        onChange={(event) =>
                          updateDefinition(definition.key, { description: event.target.value })
                        }
                      />
                    </label>
                    <label className="flex flex-col space-y-1.5 text-xs">
                      <span className="mb-0.5 font-semibold text-muted-foreground">Local path template</span>
                      <input
                        className="w-full rounded-xl border border-border/80 bg-background/50 px-3.5 py-2 text-sm font-medium text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-primary/40"
                        value={definition.localPathTemplate}
                        onChange={(event) =>
                          updateDefinition(definition.key, { localPathTemplate: event.target.value })
                        }
                      />
                    </label>
                    <label className="flex flex-col space-y-1.5 text-xs">
                      <span className="mb-0.5 font-semibold text-muted-foreground">Remote path template</span>
                      <input
                        className="w-full rounded-xl border border-border/80 bg-background/50 px-3.5 py-2 text-sm font-medium text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-primary/40"
                        value={definition.remotePathTemplate}
                        onChange={(event) =>
                          updateDefinition(definition.key, { remotePathTemplate: event.target.value })
                        }
                      />
                    </label>
                  </div>
                  <div className="mt-4 flex justify-end border-t border-border/40 pt-2">
                    <Button
                      disabled={savingKey === definition.key}
                      onClick={() => void saveDefinition(definition)}
                      type="button"
                      variant="secondary"
                      className="rounded-xl border border-border/80 bg-muted/60 transition-all duration-300 hover:bg-primary hover:text-primary-foreground cursor-pointer"
                    >
                      {savingKey === definition.key ? "Saving..." : "Save Changes"}
                    </Button>
                  </div>
                </details>
              ))}
            </div>
          </section>
        )}
      </div>
    </PageFrame>
  );
}
