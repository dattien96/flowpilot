import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";

import { PageFrame } from "@/components/common/page-frame";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { Badge } from "@/presentation/components/ui/badge";

export const Route = createFileRoute("/_authenticated/projects/")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const projects = await gateways.projectGateway.listProjects();
    return { projects };
  },
  component: ProjectsPage,
});

function ProjectsPage() {
  const { projects } = Route.useLoaderData();

  return (
    <PageFrame
      actions={
        <Link
          className="inline-flex items-center justify-center rounded-xl border border-border bg-background/50 hover:bg-muted/80 backdrop-blur-sm px-4 py-2.5 text-sm font-semibold text-card-foreground transition-all duration-300 shadow-sm"
          to="/projects/create"
        >
          Create project
        </Link>
      }
      description="Project registry with team and MCP entry points."
      title="Projects"
    >
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {projects.length === 0 ? (
          <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-6 text-sm text-muted-foreground md:col-span-2 lg:col-span-3">
            No projects yet. Create a project to get started.
          </div>
        ) : null}
        {projects.map((project) => {
          const isWeb = project.platform === "web" || project.platform === "nextjs" || project.platform === "react";
          return (
            <Link
              key={project.id}
              className="group relative flex flex-col justify-between overflow-hidden rounded-[1.8rem] border border-border/60 bg-gradient-to-b from-card/90 to-background/40 backdrop-blur-md p-6 shadow-sm hover:shadow-xl hover:border-primary/20 hover:-translate-y-1 transition-all duration-300"
              params={{ projectId: project.id }}
              to="/projects/$projectId"
            >
              {/* Dynamic top gradient line based on platform */}
              <div 
                className={`absolute top-0 left-0 right-0 h-[3px] opacity-70 group-hover:opacity-100 transition-opacity bg-gradient-to-r ${
                  isWeb 
                    ? "from-violet-500/80 via-purple-500/80 to-blue-500/80" 
                    : "from-cyan-500/80 via-teal-500/80 to-emerald-500/80"
                }`} 
              />

              <div className="flex flex-col h-full">
                {/* Header */}
                <div className="flex items-start justify-between gap-3 mb-4">
                  <div className="space-y-1">
                    <h3 className="text-xl font-bold tracking-tight text-foreground group-hover:text-primary transition-colors duration-300">
                      {project.name}
                    </h3>
                    <p className="font-mono text-[10px] tracking-wider text-muted-foreground uppercase mt-1">
                      ID: {project.id}
                    </p>
                  </div>
                  <div className="flex flex-col items-end gap-1.5 shrink-0">
                    <Badge className="rounded-full px-2.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider border border-border bg-background/50 text-foreground/80">
                      {project.platform}
                    </Badge>
                    <Badge className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold border uppercase tracking-wider ${
                      project.status === "active" 
                        ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/20" 
                        : "bg-amber-500/10 text-amber-500 border-amber-500/20"
                    }`}>
                      {project.status}
                    </Badge>
                  </div>
                </div>

                {/* Description */}
                <p className="text-sm text-muted-foreground leading-relaxed mb-5 line-clamp-3 flex-grow min-h-[3rem]">
                  {project.description}
                </p>

                {/* Details Area */}
                <div className="space-y-3 border-t border-border/40 pt-4 mb-6 text-xs text-muted-foreground/90">
                  <p className="font-medium text-foreground/75 truncate">
                    Repo: <span className="font-mono font-normal text-muted-foreground">{project.repositoryUrl}</span>
                  </p>
                  {project.directoryPath ? (
                    <p className="font-medium text-foreground/75 truncate">
                      Path: <span className="font-mono font-normal text-muted-foreground">{project.directoryPath}</span>
                    </p>
                  ) : null}
                </div>
              </div>

              {/* Actions Footer */}
              <div className="flex items-center justify-between pt-2 border-t border-border/40 mt-auto">
                <span className="text-[10px] font-mono text-muted-foreground/60">
                  Status: {project.status}
                </span>
                <span className="inline-flex items-center justify-center rounded-xl bg-muted/60 text-muted-foreground hover:bg-primary hover:text-primary-foreground border border-border/40 px-3 py-1.5 text-xs font-semibold transition-all duration-300 group/btn">
                  Open project
                  <ArrowRight className="ml-1.5 h-3.5 w-3.5 group-hover/btn:translate-x-1 transition-transform duration-300" />
                </span>
              </div>
            </Link>
          );
        })}
      </div>
    </PageFrame>
  );
}
