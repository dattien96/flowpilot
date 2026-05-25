import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { Badge } from "@/presentation/components/ui/badge";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { getTeamLinkDelta } from "./project-team-links";

type BindingDraft = {
  id: string;
  localPath: string;
  label: string;
};

export const Route = createFileRoute("/_authenticated/projects/create")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const allTeams = await gateways.teamGateway.listTeams();
    return { allTeams };
  },
  component: CreateProjectPage,
});

function CreateProjectPage() {
  const { allTeams } = Route.useLoaderData();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [form, setForm] = useState({
    name: "",
    description: "",
    platform: "android" as "android" | "ios" | "web" | "multi",
    repositoryUrl: "",
  });
  const [selectedTeamIds, setSelectedTeamIds] = useState<string[]>([]);
  const [bindings, setBindings] = useState<BindingDraft[]>([
    { id: crypto.randomUUID(), localPath: "", label: "Primary" },
  ]);

  const applyDirectorySelection = async (bindingId: string) => {
    try {
      const gateways = createGatewayBundle();
      const selection = await gateways.localRunnerGateway.pickDirectory();
      setBindings((current) =>
        current.map((binding) =>
          binding.id === bindingId ? { ...binding, localPath: selection.path } : binding,
        ),
      );
    } catch (error) {
      window.alert(
        error instanceof Error
          ? `${error.message}. Paste the project path manually if needed.`
          : "Unable to select a directory. Paste the project path manually.",
      );
    }
  };

  const createProject = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      const normalizedBindings = bindings
        .map((binding) => ({
          ...binding,
          localPath: binding.localPath.trim(),
          label: binding.label.trim(),
        }))
        .filter((binding) => binding.localPath.length > 0);

      if (normalizedBindings.length === 0) {
        throw new Error("Add at least one directory binding.");
      }

      const uniquePaths = new Set<string>();
      for (const binding of normalizedBindings) {
        if (uniquePaths.has(binding.localPath)) {
          throw new Error("Directory bindings must use unique paths.");
        }
        uniquePaths.add(binding.localPath);
      }

      const project = await gateways.projectGateway.createProject({
        name: form.name,
        description: form.description,
        platform: form.platform,
        repositoryUrl: form.repositoryUrl,
        directoryPath: normalizedBindings[0]?.localPath ?? "",
        status: "active",
        artifactStoragePreference: "supabase",
      });

      for (const binding of normalizedBindings.slice(1)) {
        await gateways.projectGateway.createProjectWorkspaceBinding(project.id, {
          localPath: binding.localPath,
          label: binding.label || null,
        });
      }

      const { toLink } = getTeamLinkDelta([], selectedTeamIds);
      await Promise.all(
        toLink.map((teamId) => gateways.teamGateway.linkTeamToProject(project.id, teamId)),
      );

      return project;
    },
    onSuccess: async (project) => {
      setForm({
        name: "",
        description: "",
        platform: "android",
        repositoryUrl: "",
      });
      setBindings([{ id: crypto.randomUUID(), localPath: "", label: "Primary" }]);
      setSelectedTeamIds([]);
      await queryClient.invalidateQueries();
      await router.navigate({ params: { projectId: project.id }, to: "/projects/$projectId" });
    },
    onError: (error) => {
      window.alert(error instanceof Error ? error.message : "Unable to create project.");
    },
  });

  const toggleTeam = (teamId: string) => {
    setSelectedTeamIds((current) =>
      current.includes(teamId)
        ? current.filter((selectedTeamId) => selectedTeamId !== teamId)
        : [...current, teamId],
    );
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    createProject.mutate();
  };

  const addBinding = () => {
    setBindings((current) => [
      ...current,
      { id: crypto.randomUUID(), localPath: "", label: "" },
    ]);
  };

  const updateBinding = (bindingId: string, patch: Partial<BindingDraft>) => {
    setBindings((current) =>
      current.map((binding) => (binding.id === bindingId ? { ...binding, ...patch } : binding)),
    );
  };

  const removeBinding = (bindingId: string) => {
    setBindings((current) => (current.length === 1 ? current : current.filter((binding) => binding.id !== bindingId)));
  };

  return (
    <PageFrame
      description="Create a new project, then optionally link teams before entering the project workspace."
      title="Create Project"
    >
      <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
        <form className="grid gap-3 md:grid-cols-2" onSubmit={handleSubmit}>
          <input
            className="rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="Project name"
            value={form.name}
            onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
            required
          />
          <input
            className="rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="https://github.com/org/repo"
            type="url"
            value={form.repositoryUrl}
            onChange={(event) =>
              setForm((current) => ({ ...current, repositoryUrl: event.target.value }))
            }
            required
          />
          <textarea
            className="min-h-28 rounded-2xl border border-border bg-card px-4 py-3 md:col-span-2"
            placeholder="Short project description"
            value={form.description}
            onChange={(event) =>
              setForm((current) => ({ ...current, description: event.target.value }))
            }
            required
          />
          <div className="rounded-[1.5rem] border border-border bg-card/80 p-4 md:col-span-2">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p className="font-medium">Directory bindings</p>
                <p className="text-sm text-muted-foreground">
                  Add one or more local project folders. The first binding will be treated as the
                  primary path for legacy compatibility.
                </p>
              </div>
              <Button type="button" variant="secondary" onClick={addBinding}>
                Add binding
              </Button>
            </div>
            <div className="mt-4 space-y-3">
              {bindings.map((binding, index) => {
                const isPrimary = index === 0;

                return (
                  <div
                    key={binding.id}
                    className="rounded-2xl border border-border bg-background px-4 py-3"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div>
                        <p className="font-medium">
                          {isPrimary ? "Primary binding" : `Binding ${index + 1}`}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          {isPrimary ? "Used as the legacy project path." : "Optional extra binding."}
                        </p>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        <Button
                          type="button"
                          variant="secondary"
                          onClick={() => void applyDirectorySelection(binding.id)}
                        >
                          Browse folder
                        </Button>
                        <Button
                          type="button"
                          variant="secondary"
                          onClick={() => removeBinding(binding.id)}
                          disabled={bindings.length === 1}
                        >
                          Remove
                        </Button>
                      </div>
                    </div>
                    <div className="mt-3 grid gap-3 md:grid-cols-2">
                      <input
                        className="rounded-2xl border border-border bg-card px-4 py-3"
                        placeholder="/Users/tiendat/Desktop/flowpilot/backend-abc"
                        value={binding.localPath}
                        onChange={(event) =>
                          updateBinding(binding.id, { localPath: event.target.value })
                        }
                        required={isPrimary}
                      />
                      <input
                        className="rounded-2xl border border-border bg-card px-4 py-3"
                        placeholder={isPrimary ? "Primary" : "Optional label"}
                        value={binding.label}
                        onChange={(event) => updateBinding(binding.id, { label: event.target.value })}
                      />
                    </div>
                  </div>
                );
              })}
            </div>
            <p className="mt-3 text-xs text-muted-foreground">
              Browse folder uses the local runner to open a native folder chooser and return the
              selected path. If that is unavailable, paste the path manually.
            </p>
          </div>
          <select
            className="rounded-2xl border border-border bg-card px-4 py-3"
            value={form.platform}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                platform: event.target.value as typeof form.platform,
              }))
            }
          >
            <option value="android">android</option>
            <option value="ios">ios</option>
            <option value="web">web</option>
            <option value="multi">multi</option>
          </select>
          <div className="rounded-[1.5rem] border border-border bg-card/70 p-4 md:col-span-2">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="font-medium">Assigned teams</p>
                <p className="text-sm text-muted-foreground">
                  Select existing teams that should be linked as soon as the project is created.
                </p>
              </div>
              <Badge>{selectedTeamIds.length} selected</Badge>
            </div>
            {allTeams.length === 0 ? (
              <p className="mt-4 text-sm text-muted-foreground">
                No teams exist yet. Create one from a project Members screen, then assign it here.
              </p>
            ) : (
              <div className="mt-4 grid gap-3 md:grid-cols-2">
                {allTeams.map((team) => {
                  const checked = selectedTeamIds.includes(team.id);

                  return (
                    <label
                      key={team.id}
                      className="flex items-start gap-3 rounded-2xl border border-border bg-background px-4 py-3"
                    >
                      <input
                        checked={checked}
                        className="mt-1"
                        onChange={() => toggleTeam(team.id)}
                        type="checkbox"
                      />
                      <span className="min-w-0">
                        <span className="block font-medium">{team.name}</span>
                        <span className="block text-sm text-muted-foreground">
                          Link this team to the new project.
                        </span>
                      </span>
                    </label>
                  );
                })}
              </div>
            )}
          </div>
          <div className="flex items-center justify-end md:col-span-2">
            <Button disabled={createProject.isPending} type="submit">
              {createProject.isPending ? "Creating..." : "Create project"}
            </Button>
          </div>
        </form>
      </section>
    </PageFrame>
  );
}
