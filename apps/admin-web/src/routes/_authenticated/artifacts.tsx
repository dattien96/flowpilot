import { useEffect, useRef, useState } from "react";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { LocalRunnerStorageDriver } from "@/domain/model/entity/local-runner";
import type { ArtifactDefinition } from "@/domain/model/entity/workflow-engine";
import { GetStorageDriverUseCase } from "@/domain/usecase/artifacts/get-storage-driver-usecase";
import { ListArtifactDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-artifact-definitions-usecase";
import { SaveArtifactDefinitionUseCase } from "@/domain/usecase/workflow-engine/save-artifact-definition-usecase";
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

  const [loading, setLoading] = useState(true);
  const [savingKey, setSavingKey] = useState<string | null>(null);
  const [storageDriver, setStorageDriver] = useState<LocalRunnerStorageDriver | null>(null);
  const [artifactDefinitions, setArtifactDefinitions] = useState<ArtifactDefinition[]>([]);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const [definitions, driver] = await Promise.all([
          listArtifactDefinitionsUseCase.current.execute(),
          getStorageDriverUseCase.current.execute(),
        ]);
        setArtifactDefinitions(definitions);
        setStorageDriver(driver);
      } catch {
        setArtifactDefinitions([]);
        setStorageDriver(null);
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
        {storageDriver ? <ArtifactStoragePanel storageDriver={storageDriver} /> : null}

        <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                Artifact Catalog
              </p>
              <h2 className="mt-3 text-2xl font-semibold tracking-tight">Definition table</h2>
              <p className="mt-2 text-sm text-muted-foreground">
                Step definitions bind to these artifact keys. Each record defines the
                local and remote path template used by workflow runs.
              </p>
            </div>
            <Link to="/artifacts/create">
              <Button disabled={loading}>Create Artifact</Button>
            </Link>
          </div>

          <div className="mt-6 space-y-3">
            {loading ? <p className="text-sm text-muted-foreground">Loading...</p> : null}
            {artifactDefinitions.map((definition) => (
              <details
                key={definition.key}
                className="rounded-2xl border border-border bg-card px-4 py-4"
              >
                <summary className="flex cursor-pointer list-none items-start justify-between gap-4">
                  <div>
                    <p className="font-semibold">{definition.name}</p>
                    <p className="text-xs text-muted-foreground">{definition.key}</p>
                    <p className="mt-2 text-xs text-muted-foreground">
                      Default file: {definition.defaultFileName || "None"}
                    </p>
                  </div>
                  <span className="text-xs font-medium text-muted-foreground">Edit</span>
                </summary>

                <div className="mt-4 grid gap-3 md:grid-cols-2">
                  <label className="space-y-2 text-sm">
                    <span className="font-medium">Name</span>
                    <input
                      className="w-full rounded-2xl border border-border bg-background px-4 py-3"
                      value={definition.name}
                      onChange={(event) =>
                        updateDefinition(definition.key, { name: event.target.value })
                      }
                    />
                  </label>
                  <label className="space-y-2 text-sm">
                    <span className="font-medium">Default file name</span>
                    <input
                      className="w-full rounded-2xl border border-border bg-background px-4 py-3"
                      value={definition.defaultFileName}
                      onChange={(event) =>
                        updateDefinition(definition.key, { defaultFileName: event.target.value })
                      }
                    />
                  </label>
                  <label className="space-y-2 text-sm md:col-span-2">
                    <span className="font-medium">Description</span>
                    <textarea
                      className="min-h-24 w-full rounded-2xl border border-border bg-background px-4 py-3"
                      value={definition.description}
                      onChange={(event) =>
                        updateDefinition(definition.key, { description: event.target.value })
                      }
                    />
                  </label>
                  <label className="space-y-2 text-sm">
                    <span className="font-medium">Local path template</span>
                    <input
                      className="w-full rounded-2xl border border-border bg-background px-4 py-3"
                      value={definition.localPathTemplate}
                      onChange={(event) =>
                        updateDefinition(definition.key, { localPathTemplate: event.target.value })
                      }
                    />
                  </label>
                  <label className="space-y-2 text-sm">
                    <span className="font-medium">Remote path template</span>
                    <input
                      className="w-full rounded-2xl border border-border bg-background px-4 py-3"
                      value={definition.remotePathTemplate}
                      onChange={(event) =>
                        updateDefinition(definition.key, { remotePathTemplate: event.target.value })
                      }
                    />
                  </label>
                </div>
                <div className="mt-4 flex justify-end">
                  <Button
                    disabled={savingKey === definition.key}
                    onClick={() => void saveDefinition(definition)}
                    type="button"
                    variant="secondary"
                  >
                    {savingKey === definition.key ? "Saving..." : "Save"}
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
