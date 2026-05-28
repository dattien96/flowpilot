import { useEffect, useMemo, useRef, useState } from "react";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";
import { ArrowRight, Star } from "lucide-react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { ListWorkflowsUseCase } from "@/domain/usecase/workflow-engine/list-workflows-usecase";
import type { Workflow } from "@/domain/model/entity/workflow-engine";
import type { Project } from "@/domain/model/entity/project";

type WorkflowSortOption = "name-asc" | "name-desc" | "updated-asc" | "updated-desc";

export const Route = createFileRoute("/_authenticated/workflows")({
  validateSearch: (search: Record<string, unknown>) => ({
    projectId: typeof search.projectId === "string" ? search.projectId : undefined,
  }),
  component: WorkflowDefinitionsPage,
});

export function WorkflowDefinitionsPage() {
  const location = useLocation();
  const { projectId: searchProjectId } = Route.useSearch();
  const gatewayBundle = useRef(createGatewayBundle());
  const listWorkflowsUseCase = useRef(
    new ListWorkflowsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [scopeFilter, setScopeFilter] = useState<"all" | "global" | "private">("all");
  const [projectFilter, setProjectFilter] = useState(searchProjectId ?? "all");
  const [sortBy, setSortBy] = useState<WorkflowSortOption>("updated-desc");
  const [favorites, setFavorites] = useState<Set<string>>(new Set());

  const loadFavorites = async () => {
    try {
      const favs = await gatewayBundle.current.userFavoriteGateway.listFavorites();
      setFavorites(new Set(favs.filter(f => f.targetType === "workflow").map(f => f.targetId)));
    } catch (e) {
      console.error("Failed to load favorites", e);
    }
  };

  const toggleFavorite = async (workflowId: string) => {
    const isCurrentlyFav = favorites.has(workflowId);
    try {
      await gatewayBundle.current.userFavoriteGateway.toggleFavorite("workflow", workflowId, !isCurrentlyFav);
      const next = new Set(favorites);
      if (!isCurrentlyFav) next.add(workflowId);
      else next.delete(workflowId);
      setFavorites(next);
    } catch (e) {
      console.error("Failed to toggle favorite", e);
    }
  };

  useEffect(() => {
    setProjectFilter(searchProjectId ?? "all");
  }, [searchProjectId]);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const [workflowRows, projectRows] = await Promise.all([
          listWorkflowsUseCase.current.execute(),
          gatewayBundle.current.projectGateway.listProjects(),
        ]);
        setWorkflows(workflowRows);
        setProjects(projectRows);
        await loadFavorites();
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, []);

  const projectNameById = useMemo(
    () => new Map(projects.map((project) => [project.id, project.name])),
    [projects]
  );

  const visibleWorkflows = useMemo(() => {
    const filtered = workflows.filter((workflow) => {
      const matchesScope =
        scopeFilter === "all" ||
        (scopeFilter === "global" && workflow.projectId === null) ||
        (scopeFilter === "private" && workflow.projectId !== null);
      const matchesProject =
        projectFilter === "all" || workflow.projectId === projectFilter;

      return matchesScope && matchesProject;
    });

    return filtered.slice().sort((left, right) => {
      switch (sortBy) {
        case "name-asc":
          return left.name.localeCompare(right.name);
        case "name-desc":
          return right.name.localeCompare(left.name);
        case "updated-asc":
          return new Date(left.updatedAt).getTime() - new Date(right.updatedAt).getTime();
        case "updated-desc":
        default:
          return new Date(right.updatedAt).getTime() - new Date(left.updatedAt).getTime();
      }
    });
  }, [projectFilter, scopeFilter, sortBy, workflows]);

  if (location.pathname !== "/workflows") {
    return <Outlet />;
  }

  return (
    <PageFrame
      title="Workflow Definitions"
      description="Browse all workflow definitions, including built-in global templates and private project-owned workflows."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/workflow-steps">
            <Button variant="secondary" className="rounded-xl border border-border bg-background/50 hover:bg-muted/80 backdrop-blur-sm transition-all duration-300">
              Step catalog
            </Button>
          </Link>
          <Link to="/workflow-runs">
            <Button variant="secondary" className="rounded-xl border border-border bg-background/50 hover:bg-muted/80 backdrop-blur-sm transition-all duration-300">
              Run history
            </Button>
          </Link>
          <Link to="/workflows/create" search={{ projectId: searchProjectId }}>
            <Button className="rounded-xl shadow-sm hover:shadow-md hover:opacity-90 transition-all duration-300">
              Create workflow
            </Button>
          </Link>
        </div>
      }
    >
      <div className="grid gap-4 rounded-[1.5rem] border border-border/60 bg-card/45 p-5 backdrop-blur-sm md:grid-cols-3">
        <label className="space-y-2 text-sm flex flex-col">
          <span className="font-semibold text-muted-foreground mb-1">Scope</span>
          <select
            className="w-full rounded-xl border border-border/80 bg-background/50 px-4 py-2.5 focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all cursor-pointer font-medium"
            value={scopeFilter}
            onChange={(event) => setScopeFilter(event.target.value as typeof scopeFilter)}
          >
            <option value="all">All definitions</option>
            <option value="global">Global only</option>
            <option value="private">Private only</option>
          </select>
        </label>
        <label className="space-y-2 text-sm flex flex-col">
          <span className="font-semibold text-muted-foreground mb-1">Owner project</span>
          <select
            className="w-full rounded-xl border border-border/80 bg-background/50 px-4 py-2.5 focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all cursor-pointer font-medium"
            value={projectFilter}
            onChange={(event) => setProjectFilter(event.target.value)}
          >
            <option value="all">All projects</option>
            {projects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </select>
        </label>
        <label className="space-y-2 text-sm flex flex-col">
          <span className="font-semibold text-muted-foreground mb-1">Sort by</span>
          <select
            className="w-full rounded-xl border border-border/80 bg-background/50 px-4 py-2.5 focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all cursor-pointer font-medium"
            value={sortBy}
            onChange={(event) => setSortBy(event.target.value as WorkflowSortOption)}
          >
            <option value="name-asc">Alphabet (A-Z)</option>
            <option value="name-desc">Alphabet (Z-A)</option>
            <option value="updated-desc">Updated time (Newest first)</option>
            <option value="updated-asc">Updated time (Oldest first)</option>
          </select>
        </label>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {loading ? (
          <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-5 text-sm text-muted-foreground md:col-span-2 lg:col-span-3">
            Loading workflow definitions...
          </div>
        ) : null}
        {!loading && visibleWorkflows.length === 0 ? (
          <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-5 text-sm text-muted-foreground md:col-span-2 lg:col-span-3">
            No workflow definitions match the current filters.
          </div>
        ) : null}
        {visibleWorkflows.map((workflow) => {
          const isBuiltIn = workflow.isTemplate;
          const isPrivate = !!workflow.projectId;
          const isFav = favorites.has(workflow.id);
          return (
            <div
              key={workflow.id}
              className="group relative flex flex-col justify-between overflow-hidden rounded-[1.8rem] border border-border/60 bg-gradient-to-b from-card/90 to-background/40 backdrop-blur-md p-6 shadow-sm hover:shadow-xl hover:border-primary/20 hover:-translate-y-1 transition-all duration-300"
            >
              {/* Dynamic top gradient line based on workflow type */}
              <div 
                className={`absolute top-0 left-0 right-0 h-[3px] opacity-70 group-hover:opacity-100 transition-opacity bg-gradient-to-r ${
                  isBuiltIn 
                    ? "from-emerald-500/80 via-teal-500/80 to-cyan-500/80" 
                    : "from-violet-500/80 via-purple-500/80 to-blue-500/80"
                }`} 
              />

              <div className="flex flex-col h-full">
                {/* Header */}
                <div className="flex items-start justify-between gap-3 mb-4">
                  <div className="space-y-1">
                    <h3 className="text-xl font-bold tracking-tight text-foreground group-hover:text-primary transition-colors duration-300">
                      {workflow.name}
                    </h3>
                    <p className="font-mono text-[10px] tracking-wider text-muted-foreground uppercase mt-1">
                      ID: {workflow.id}
                    </p>
                  </div>
                  <div className="flex flex-col items-end gap-1 shrink-0">
                    <button
                      onClick={(e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        void toggleFavorite(workflow.id);
                      }}
                      className={`mb-1 transition-colors ${isFav ? 'text-yellow-400 hover:text-yellow-500' : 'text-muted-foreground/40 hover:text-yellow-400/70'}`}
                      title={isFav ? "Remove from quick run" : "Add to quick run"}
                    >
                      <Star className="h-5 w-5" fill={isFav ? "currentColor" : "none"} />
                    </button>
                    {isBuiltIn ? (
                      <span className="rounded-full px-2 py-0.5 text-[10px] font-semibold bg-emerald-500/10 text-emerald-500 border border-emerald-500/20 uppercase tracking-wider">
                        Built-in
                      </span>
                    ) : null}
                    <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold border uppercase tracking-wider ${
                      isPrivate 
                        ? "bg-violet-500/10 text-violet-500 border-violet-500/20" 
                        : "bg-muted/50 text-muted-foreground border-border/50"
                    }`}>
                      {isPrivate ? "Private" : "Global"}
                    </span>
                  </div>
                </div>

                {/* Description */}
                <p className="text-sm text-muted-foreground leading-relaxed mb-5 line-clamp-3 flex-grow min-h-[3rem]">
                  {workflow.description}
                </p>

                {/* Details Area */}
                <div className="space-y-3 border-t border-border/40 pt-4 mb-6 text-xs text-muted-foreground/90">
                  <p className="font-medium text-foreground/75">
                    Owner: {workflow.projectId ? projectNameById.get(workflow.projectId) ?? workflow.projectId : "Workspace global"}
                  </p>
                  <p className="font-medium text-foreground/75 flex justify-between items-center">
                    <span>Flow Steps:</span>
                    <span className="font-semibold text-foreground/80 bg-muted/60 px-2 py-0.5 rounded border border-border/30">
                      {workflow.steps?.length ?? 0} {workflow.steps?.length === 1 ? "step" : "steps"}
                    </span>
                  </p>
                </div>
              </div>

              {/* Actions Footer */}
              <div className="flex items-center justify-between pt-2 border-t border-border/40 mt-auto">
                <span className="text-[10px] font-mono text-muted-foreground/60">
                  By: {workflow.createdBy || "system"}
                </span>
                <Link
                  params={{ workflowId: workflow.id }}
                  search={{ projectId: projectFilter !== "all" ? projectFilter : undefined }}
                  to="/workflows/$workflowId"
                >
                  <Button size="sm" variant="secondary" className="rounded-xl bg-muted/60 hover:bg-primary hover:text-primary-foreground border border-border/40 transition-all duration-300 group/btn">
                    Open workflow
                    <ArrowRight className="ml-1.5 h-3.5 w-3.5 group-hover/btn:translate-x-1 transition-transform duration-300" />
                  </Button>
                </Link>
              </div>
            </div>
          );
        })}
      </div>
    </PageFrame>
  );
}
