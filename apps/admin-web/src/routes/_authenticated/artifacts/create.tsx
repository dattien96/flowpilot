import { useRef, useState } from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { ArtifactDefinition } from "@/domain/model/entity/workflow-engine";
import { SaveArtifactDefinitionUseCase } from "@/domain/usecase/workflow-engine/save-artifact-definition-usecase";

export const Route = createFileRoute("/_authenticated/artifacts/create")({
  component: CreateArtifactPage,
});

function emptyDefinition(): ArtifactDefinition {
  return {
    key: "",
    name: "",
    description: "",
    localPathTemplate: "",
    remotePathTemplate: "",
    defaultFileName: "",
    createdAt: "",
    updatedAt: "",
  };
}

export function CreateArtifactPage() {
  const navigate = useNavigate();
  const gatewayBundle = useRef(createGatewayBundle());
  const saveArtifactDefinitionUseCase = useRef(
    new SaveArtifactDefinitionUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const [saving, setSaving] = useState(false);
  const [definition, setDefinition] = useState<ArtifactDefinition>(emptyDefinition());

  const save = async () => {
    if (!definition.key.trim() || !definition.name.trim()) {
      window.alert("Artifact key and name are required.");
      return;
    }

    setSaving(true);
    try {
      await saveArtifactDefinitionUseCase.current.execute(definition);
      await navigate({ to: "/artifacts" as never });
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to save artifact definition.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <PageFrame
      title="Create Artifact"
      description="Create a reusable artifact definition that workflow steps can use as input or output."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/artifacts">
            <Button variant="secondary">Back to artifacts</Button>
          </Link>
          <Button disabled={saving} onClick={() => void save()}>
            {saving ? "Saving..." : "Create Artifact"}
          </Button>
        </div>
      }
    >
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Artifact Catalog
          </p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight">New artifact definition</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            Define the reusable key, display name, and storage path templates for this artifact.
          </p>
        </div>

        <div className="mt-6 grid gap-3 md:grid-cols-2">
          <label className="space-y-2 text-sm">
            <span className="font-medium">Key</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              value={definition.key}
              onChange={(event) =>
                setDefinition((current) => ({ ...current, key: event.target.value }))
              }
              placeholder="plan_artifact"
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Name</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              value={definition.name}
              onChange={(event) =>
                setDefinition((current) => ({ ...current, name: event.target.value }))
              }
              placeholder="Plan"
            />
          </label>
          <label className="space-y-2 text-sm md:col-span-2">
            <span className="font-medium">Description</span>
            <textarea
              className="min-h-24 w-full rounded-2xl border border-border bg-background px-4 py-3"
              value={definition.description}
              onChange={(event) =>
                setDefinition((current) => ({
                  ...current,
                  description: event.target.value,
                }))
              }
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Local path template</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              value={definition.localPathTemplate}
              onChange={(event) =>
                setDefinition((current) => ({
                  ...current,
                  localPathTemplate: event.target.value,
                }))
              }
              placeholder=".flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/Plan.md"
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Remote path template</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              value={definition.remotePathTemplate}
              onChange={(event) =>
                setDefinition((current) => ({
                  ...current,
                  remotePathTemplate: event.target.value,
                }))
              }
              placeholder="artifacts/{projectId}/{workflowRunId}/{stepType}/Plan.md"
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Default file name</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              value={definition.defaultFileName}
              onChange={(event) =>
                setDefinition((current) => ({
                  ...current,
                  defaultFileName: event.target.value,
                }))
              }
              placeholder="Plan.md"
            />
          </label>
        </div>
      </section>
    </PageFrame>
  );
}
