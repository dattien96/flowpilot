import { useEffect, useRef, useState } from "react";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { LocalRunnerStorageDriver } from "@/domain/model/entity/local-runner";
import type { Project } from "@/domain/model/entity/project";
import { ListProjectsUseCase } from "@/domain/usecase/projects/list-projects-usecase";
import type { ArtifactDefinition } from "@/domain/model/entity/workflow-engine";
import { GetStorageDriverUseCase } from "@/domain/usecase/artifacts/get-storage-driver-usecase";
import { ListArtifactDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-artifact-definitions-usecase";
import { SaveArtifactDefinitionUseCase } from "@/domain/usecase/workflow-engine/save-artifact-definition-usecase";
import { ArtifactCloudStoragePanel } from "@/presentation/components/artifacts/artifact-cloud-storage-panel";
import { ArtifactStoragePanel } from "@/presentation/components/artifacts/artifact-storage-panel";

export const Route = createFileRoute("/_authenticated/artifacts")({
  component: ArtifactsPage,
});

export function ArtifactsPage() {
  const location = useLocation();
  const gatewayBundle = useRef(createGatewayBundle());
  const listArtifactDefinitionsUseCase = useRef(
    new ListArtifactDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const saveArtifactDefinitionUseCase = useRef(
    new SaveArtifactDefinitionUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const getStorageDriverUseCase = useRef(
    new GetStorageDriverUseCase(gatewayBundle.current.localRunnerGateway)
  );
  const listProjectsUseCase = useRef(
    new ListProjectsUseCase(gatewayBundle.current.projectGateway)
  );

  const [loading, setLoading] = useState(true);
  const [savingKey, setSavingKey] = useState<string | null>(null);
  const [storageDriver, setStorageDriver] = useState<LocalRunnerStorageDriver | null>(null);
  const [artifactDefinitions, setArtifactDefinitions] = useState<ArtifactDefinition[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const [definitions, driver, loadedProjects] = await Promise.all([
          listArtifactDefinitionsUseCase.current.execute(),
          getStorageDriverUseCase.current.execute(),
          listProjectsUseCase.current.execute(),
        ]);
        setArtifactDefinitions(definitions);
        setStorageDriver(driver);
        setProjects(loadedProjects);
      } catch {
        setArtifactDefinitions([]);
        setStorageDriver(null);
        setProjects([]);
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, []);

  const updateDefinition = (key: string, patch: Partial<ArtifactDefinition>) => {
    setArtifactDefinitions((current) =>
      current.map((definition) =>
        definition.key === key ? { ...definition, ...patch } : definition
      )
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
      description="Manage the global artifact catalog and storage driver configuration."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/settings/runner">
            <Button variant="secondary">Runner</Button>
          </Link>
          <Link to="/settings/mcp-servers">
            <Button variant="secondary">MCP Servers</Button>
          </Link>
        </div>
      }
    >
      <div className="space-y-6">
        {projects.length > 0 ? <ArtifactCloudStoragePanel projects={projects} /> : null}
        {storageDriver ? <ArtifactStoragePanel storageDriver={storageDriver} /> : null}

        <section className="rounded-[1.6rem] border border-border/60 bg-card/45 p-6 backdrop-blur-sm shadow-sm">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-primary/80 font-semibold">
                Artifact Catalog
              </p>
              <h2 className="mt-3 text-2xl font-bold tracking-tight">Definition catalog</h2>
              <p className="mt-2 text-sm text-muted-foreground leading-relaxed">
                Step definitions bind to these artifact keys. Each record defines the
                local and remote path template used by workflow runs.
              </p>
            </div>
            <Link to="/artifacts/create">
              <Button disabled={loading} className="rounded-xl shadow-sm hover:shadow-md hover:opacity-90 transition-all duration-300">
                Create Artifact
              </Button>
            </Link>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 mt-6">
            {loading ? (
              <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-5 text-sm text-muted-foreground md:col-span-2 lg:col-span-3">
                Loading...
              </div>
            ) : null}
            {artifactDefinitions.map((definition) => (
              <details
                key={definition.key}
                className="group relative flex flex-col justify-between overflow-hidden rounded-[1.8rem] border border-border/60 bg-gradient-to-b from-card/90 to-background/40 backdrop-blur-md p-6 shadow-sm hover:shadow-xl hover:border-primary/20 transition-all duration-300 [&[open]]:shadow-lg [&[open]]:border-primary/30"
              >
                {/* Dynamic top gradient line */}
                <div className="absolute top-0 left-0 right-0 h-[3px] bg-gradient-to-r from-violet-500/85 via-purple-500/85 to-blue-500/85 opacity-50 group-hover:opacity-100 group-[[open]]:opacity-100 transition-opacity" />

                <summary className="flex cursor-pointer list-none items-start justify-between gap-4 outline-none">
                  <div className="space-y-1 min-w-0">
                    <p className="text-lg font-bold tracking-tight text-foreground group-hover:text-primary transition-colors duration-300 truncate">
                      {definition.name}
                    </p>
                    <p className="font-mono text-[10px] tracking-wider text-muted-foreground uppercase">
                      Key: {definition.key}
                    </p>
                    <p className="mt-2 text-xs text-muted-foreground/80 font-medium">
                      Default file: <span className="font-mono bg-muted/60 px-1.5 py-0.5 rounded text-[11px] border border-border/30">{definition.defaultFileName || "None"}</span>
                    </p>
                  </div>
                  <span className="text-[10px] shrink-0 font-semibold uppercase tracking-wider px-2 py-0.5 rounded border border-border bg-muted/30 text-muted-foreground/80 group-hover:bg-primary group-hover:text-primary-foreground group-hover:border-primary/20 transition-all duration-300">
                    Edit
                  </span>
                </summary>

                <div className="mt-6 pt-4 border-t border-border/40 grid gap-4">
                  <label className="space-y-1.5 text-xs flex flex-col">
                    <span className="font-semibold text-muted-foreground mb-0.5">Name</span>
                    <input
                      className="w-full rounded-xl border border-border/80 bg-background/50 px-3.5 py-2 focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all font-medium text-sm text-foreground"
                      value={definition.name}
                      onChange={(event) =>
                        updateDefinition(definition.key, { name: event.target.value })
                      }
                    />
                  </label>
                  <label className="space-y-1.5 text-xs flex flex-col">
                    <span className="font-semibold text-muted-foreground mb-0.5">Default file name</span>
                    <input
                      className="w-full rounded-xl border border-border/80 bg-background/50 px-3.5 py-2 focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all font-medium text-sm text-foreground"
                      value={definition.defaultFileName}
                      onChange={(event) =>
                        updateDefinition(definition.key, { defaultFileName: event.target.value })
                      }
                    />
                  </label>
                  <label className="space-y-1.5 text-xs flex flex-col">
                    <span className="font-semibold text-muted-foreground mb-0.5">Description</span>
                    <textarea
                      className="min-h-20 w-full rounded-xl border border-border/80 bg-background/50 px-3.5 py-2 focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all font-medium text-sm text-foreground resize-y"
                      value={definition.description}
                      onChange={(event) =>
                        updateDefinition(definition.key, { description: event.target.value })
                      }
                    />
                  </label>
                  <label className="space-y-1.5 text-xs flex flex-col">
                    <span className="font-semibold text-muted-foreground mb-0.5">Local path template</span>
                    <input
                      className="w-full rounded-xl border border-border/80 bg-background/50 px-3.5 py-2 focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all font-medium text-sm text-foreground"
                      value={definition.localPathTemplate}
                      onChange={(event) =>
                        updateDefinition(definition.key, { localPathTemplate: event.target.value })
                      }
                    />
                  </label>
                  <label className="space-y-1.5 text-xs flex flex-col">
                    <span className="font-semibold text-muted-foreground mb-0.5">Remote path template</span>
                    <input
                      className="w-full rounded-xl border border-border/80 bg-background/50 px-3.5 py-2 focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all font-medium text-sm text-foreground"
                      value={definition.remotePathTemplate}
                      onChange={(event) =>
                        updateDefinition(definition.key, { remotePathTemplate: event.target.value })
                      }
                    />
                  </label>
                </div>
                <div className="mt-4 flex justify-end pt-2 border-t border-border/40">
                  <Button
                    disabled={savingKey === definition.key}
                    onClick={() => void saveDefinition(definition)}
                    type="button"
                    variant="secondary"
                    className="rounded-xl border border-border/80 bg-muted/60 hover:bg-primary hover:text-primary-foreground transition-all duration-300"
                  >
                    {savingKey === definition.key ? "Saving..." : "Save Changes"}
                  </Button>
                </div>
              </details>
            ))}
          </div>
        </section>
      </div>
    </PageFrame>
  );
}
