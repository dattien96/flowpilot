import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { ProjectWorkspaceBinding } from "@/domain/model/entity/project-workspace-binding";
import { autoInitProjectEngine } from "@/features/projects/project-engine-auto-init";
import { Badge } from "@/presentation/components/ui/badge";

type BindingDraft = {
  id: string;
  localPath: string;
  label: string;
  persisted: boolean;
};

function toDraft(binding: ProjectWorkspaceBinding): BindingDraft {
  return {
    id: binding.id,
    localPath: binding.localPath,
    label: binding.label ?? "",
    persisted: true,
  };
}

export const Route = createFileRoute("/_authenticated/projects/$projectId/directory-bindings")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const [project, bindings] = await Promise.all([
      gateways.projectGateway.getProjectById(params.projectId),
      gateways.projectGateway.listProjectWorkspaceBindings(params.projectId),
    ]);

    return { project, bindings };
  },
  component: ProjectDirectoryBindingsPage,
});

function ProjectDirectoryBindingsPage() {
  const { project, bindings } = Route.useLoaderData();

  if (!project) {
    return null;
  }

  return <ProjectDirectoryBindingsContent projectId={project.id} bindings={bindings} />;
}

function ProjectDirectoryBindingsContent({
  bindings,
  projectId,
}: {
  bindings: ProjectWorkspaceBinding[];
  projectId: string;
}) {
  const gatewayBundle = createGatewayBundle();
  const [drafts, setDrafts] = useState<BindingDraft[]>(() => bindings.map(toDraft));
  const [savingBindingId, setSavingBindingId] = useState<string | null>(null);

  const addBinding = () => {
    setDrafts((current) => [
      ...current,
      {
        id: crypto.randomUUID(),
        localPath: "",
        label: "",
        persisted: false,
      },
    ]);
  };

  const updateDraft = (bindingId: string, patch: Partial<BindingDraft>) => {
    setDrafts((current) =>
      current.map((binding) => (binding.id === bindingId ? { ...binding, ...patch } : binding)),
    );
  };

  const removeDraft = async (bindingId: string) => {
    const target = drafts.find((binding) => binding.id === bindingId);
    if (!target) {
      return;
    }

    if (!target.persisted) {
      setDrafts((current) => current.filter((binding) => binding.id !== bindingId));
      return;
    }

    if (!window.confirm("Delete this directory binding?")) {
      return;
    }

    setSavingBindingId(bindingId);
    try {
      await gatewayBundle.projectGateway.deleteProjectWorkspaceBinding(bindingId);
      setDrafts((current) => current.filter((binding) => binding.id !== bindingId));
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to delete directory binding.");
    } finally {
      setSavingBindingId(null);
    }
  };

  const browseForBinding = async (bindingId: string) => {
    try {
      const selection = await gatewayBundle.localRunnerGateway.pickDirectory();
      updateDraft(bindingId, { localPath: selection.path });
    } catch (error) {
      window.alert(
        error instanceof Error
          ? `${error.message}. Paste the project path manually if needed.`
          : "Unable to select a directory. Paste the project path manually.",
      );
    }
  };

  const saveDraft = async (bindingId: string) => {
    const target = drafts.find((binding) => binding.id === bindingId);
    if (!target) {
      return;
    }

    const localPath = target.localPath.trim();
    const label = target.label.trim();
    if (!localPath) {
      window.alert("Directory path is required.");
      return;
    }

    setSavingBindingId(bindingId);
    try {
      if (target.persisted) {
        const updated = await gatewayBundle.projectGateway.updateProjectWorkspaceBinding(bindingId, {
          localPath,
          label: label || null,
        });
        setDrafts((current) =>
          current.map((binding) =>
            binding.id === bindingId
              ? {
                  ...binding,
                  id: updated.id,
                  localPath: updated.localPath,
                  label: updated.label ?? "",
                  persisted: true,
                }
              : binding,
          ),
        );
        void autoInitProjectEngine(projectId, updated.localPath);
        return;
      }

      const created = await gatewayBundle.projectGateway.createProjectWorkspaceBinding(projectId, {
        localPath,
        label: label || null,
      });
      setDrafts((current) =>
        current.map((binding) =>
          binding.id === bindingId
            ? {
                ...binding,
                id: created.id,
                localPath: created.localPath,
                label: created.label ?? "",
                persisted: true,
              }
            : binding,
        ),
      );
      void autoInitProjectEngine(projectId, created.localPath);
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to save directory binding.");
    } finally {
      setSavingBindingId(null);
    }
  };

  return (
    <PageFrame
      description="Manage the local directories that this shared project can run against."
      title="Directory Binding"
    >
      <div className="space-y-6">
        <ProjectSectionNav projectId={projectId} />

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <h2 className="text-xl font-semibold">Project directory bindings</h2>
              <p className="mt-2 text-sm text-muted-foreground">
                Add one or more local folders here. The runner will use a valid bound path when
                workflow execution starts.
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Badge>{drafts.length} bindings</Badge>
              <Button type="button" variant="secondary" onClick={addBinding}>
                Add binding
              </Button>
            </div>
          </div>

          <div className="mt-5 space-y-3">
            {drafts.length === 0 ? (
              <div className="rounded-2xl border border-dashed border-border bg-card px-4 py-8 text-sm text-muted-foreground">
                No directory bindings yet. Add one to make this project runnable on this machine.
              </div>
            ) : null}

            {drafts.map((binding, index) => (
              <div
                key={binding.id}
                className="rounded-2xl border border-border bg-card px-4 py-4"
              >
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div>
                    <p className="font-medium">
                      {index === 0 ? "Primary binding" : `Binding ${index + 1}`}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {binding.persisted ? "Saved" : "New"} binding for this project.
                    </p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={() => void browseForBinding(binding.id)}
                    >
                      Browse folder
                    </Button>
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={() => void saveDraft(binding.id)}
                      disabled={savingBindingId === binding.id}
                    >
                      {savingBindingId === binding.id ? "Saving..." : "Save"}
                    </Button>
                    <Button
                      type="button"
                      variant="secondary"
                      className="text-red-600"
                      onClick={() => void removeDraft(binding.id)}
                      disabled={savingBindingId === binding.id}
                    >
                      Delete
                    </Button>
                  </div>
                </div>

                <div className="mt-4 grid gap-3 md:grid-cols-2">
                  <input
                    className="rounded-2xl border border-border bg-background px-4 py-3"
                    placeholder="/Users/tiendat/Desktop/flowpilot/backend-abc"
                    value={binding.localPath}
                    onChange={(event) =>
                      updateDraft(binding.id, { localPath: event.target.value })
                    }
                  />
                  <input
                    className="rounded-2xl border border-border bg-background px-4 py-3"
                    placeholder="Primary"
                    value={binding.label}
                    onChange={(event) => updateDraft(binding.id, { label: event.target.value })}
                  />
                </div>
              </div>
            ))}
          </div>
        </section>
      </div>
    </PageFrame>
  );
}
