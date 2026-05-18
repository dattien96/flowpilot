import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate, useRouter } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";

import { createGatewayBundle } from "@/data/repository/browser-factory";
import { Button } from "@/components/ui/button";
import { PageFrame } from "@/components/common/page-frame";
import { Badge } from "@/presentation/components/ui/badge";

export const Route = createFileRoute("/_authenticated/projects/")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    return gateways.projectGateway.listProjects();
  },
  component: ProjectsPage,
});

function ProjectsPage() {
  const projects = Route.useLoaderData();
  const router = useRouter();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [form, setForm] = useState({
    name: "",
    description: "",
    platform: "android" as "android" | "ios" | "web" | "multi",
    repositoryUrl: "",
    directoryPath: "",
  });

  const createProject = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      return gateways.projectGateway.createProject({
        name: form.name,
        description: form.description,
        platform: form.platform,
        repositoryUrl: form.repositoryUrl,
        directoryPath: form.directoryPath || null,
        status: "active",
        artifactStoragePreference: "supabase",
      });
    },
    onSuccess: async () => {
      setForm({
        name: "",
        description: "",
        platform: "android",
        repositoryUrl: "",
        directoryPath: "",
      });
      await queryClient.invalidateQueries();
      await router.invalidate();
    },
  });

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    createProject.mutate();
  };

  return (
    <PageFrame
      description="Project registry with management metadata and route entry points for team-oriented project work."
      title="Projects"
    >
      <section className="mb-6 rounded-[1.5rem] border border-border bg-background/60 p-5">
        <h2 className="text-xl font-semibold">Create project</h2>
        <form className="mt-4 grid gap-3 md:grid-cols-2" onSubmit={handleSubmit}>
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
          <input
            className="rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="Optional directory path"
            value={form.directoryPath}
            onChange={(event) =>
              setForm((current) => ({ ...current, directoryPath: event.target.value }))
            }
          />
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
          <div className="flex items-center justify-end md:col-span-2">
            <Button type="submit">Create project</Button>
          </div>
        </form>
      </section>

      <div className="grid gap-4 xl:grid-cols-2">
        {projects.map((project) => (
          <Link
            key={project.id}
            className="rounded-[1.5rem] border border-border bg-background/60 p-5 transition-transform hover:-translate-y-0.5"
            params={{ projectId: project.id }}
            to="/projects/$projectId"
          >
            <div className="flex items-start justify-between gap-4">
              <div>
                <h3 className="text-xl font-semibold">{project.name}</h3>
                <p className="mt-2 text-sm text-muted-foreground">{project.description}</p>
              </div>
              <div className="flex flex-col items-end gap-2">
                <Badge>{project.platform}</Badge>
                <Badge>{project.status}</Badge>
              </div>
            </div>
            <p className="mt-4 text-xs text-muted-foreground">{project.repositoryUrl}</p>
            {project.directoryPath ? (
              <p className="mt-2 text-xs text-muted-foreground">
                Directory: {project.directoryPath}
              </p>
            ) : null}
            <Button className="mt-4" variant="secondary">
              Open project
            </Button>
          </Link>
        ))}
      </div>
    </PageFrame>
  );
}
