import Link from "next/link";

import { createGatewayBundle } from "@/data/repository/factory";
import { ListProjectsUseCase } from "@/domain/usecase/projects/list-projects-usecase";
import { Button } from "@/presentation/components/ui/button";
import { Badge } from "@/presentation/components/ui/badge";

export default async function ProjectsPage() {
  const gateways = await createGatewayBundle();
  const projects = await new ListProjectsUseCase(gateways.projectGateway).execute();

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Projects
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">Project registry</h1>
      </header>
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <h2 className="text-xl font-semibold">Create project</h2>
        <form action="/api/projects" method="post" className="mt-4 grid gap-3 lg:grid-cols-2">
          <input
            className="rounded-2xl border border-border bg-card px-4 py-3"
            name="name"
            placeholder="Project name"
            required
          />
          <input
            className="rounded-2xl border border-border bg-card px-4 py-3"
            name="repositoryUrl"
            placeholder="https://github.com/org/repo"
            required
            type="url"
          />
          <textarea
            className="min-h-28 rounded-2xl border border-border bg-card px-4 py-3 lg:col-span-2"
            name="description"
            placeholder="Short project description"
            required
          />
          <select
            className="rounded-2xl border border-border bg-card px-4 py-3"
            defaultValue="android"
            name="platform"
          >
            <option value="android">android</option>
            <option value="ios">ios</option>
            <option value="web">web</option>
            <option value="multi">multi</option>
          </select>
          <div className="flex items-center justify-end">
            <Button type="submit">Create project</Button>
          </div>
        </form>
      </section>
      <div className="grid gap-4 xl:grid-cols-2">
        {projects.map((project) => (
          <Link
            key={project.id}
            className="rounded-[1.6rem] border border-border bg-background/70 p-6 transition-transform hover:-translate-y-0.5"
            href={`/projects/${project.id}`}
          >
            <div className="flex items-start justify-between gap-4">
              <div>
                <h2 className="text-2xl font-semibold">{project.name}</h2>
                <p className="mt-2 text-sm text-muted-foreground">{project.description}</p>
              </div>
              <Badge>{project.platform}</Badge>
            </div>
            <p className="mt-6 font-mono text-xs uppercase tracking-[0.24em] text-muted-foreground">
              {project.repositoryUrl}
            </p>
          </Link>
        ))}
      </div>
    </div>
  );
}
