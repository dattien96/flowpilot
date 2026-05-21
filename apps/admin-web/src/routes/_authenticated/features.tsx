import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { Feature } from "@/domain/model/entity/feature";
import type { Project } from "@/domain/model/entity/project";
import { ListFeaturesUseCase } from "@/domain/usecase/features/list-features-usecase";
import { ListProjectsUseCase } from "@/domain/usecase/projects/list-projects-usecase";
import { Badge } from "@/presentation/components/ui/badge";

export const Route = createFileRoute("/_authenticated/features")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const [features, projects] = await Promise.all([
      new ListFeaturesUseCase(gateways.featureGateway).execute(),
      new ListProjectsUseCase(gateways.projectGateway).execute(),
    ]);

    return { features, projects };
  },
  component: FeaturesPage,
});

export function FeaturesPage() {
  const location = useLocation();
  const { features, projects } = Route.useLoaderData();

  if (location.pathname !== "/features") {
    return <Outlet />;
  }

  return <FeaturesContent features={features} projects={projects} />;
}

export function FeaturesContent({
  features,
  projects,
}: {
  features: Feature[];
  projects: Project[];
}) {
  const projectNameById = new Map(projects.map((project) => [project.id, project.name]));

  return (
    <PageFrame
      title="Features"
      description="Browse feature intake items, inspect their context, and launch the demo workflow from the feature detail page."
    >
      <div className="grid gap-4 xl:grid-cols-2">
        {features.length === 0 ? (
          <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-5 text-sm text-muted-foreground xl:col-span-2">
            No features exist yet.
          </div>
        ) : null}

        {features.map((feature) => (
          <Link
            key={feature.id}
            className="block rounded-[1.5rem] border border-border bg-background/60 p-5 transition-colors hover:bg-background"
            params={{ featureId: feature.id }}
            to="/features/$featureId"
          >
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 className="text-xl font-semibold">{feature.title}</h2>
                <p className="mt-2 text-sm text-muted-foreground">{feature.userProblem}</p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Badge>{feature.priority}</Badge>
                <Badge>{feature.status}</Badge>
              </div>
            </div>
            <p className="mt-4 text-xs uppercase tracking-[0.18em] text-muted-foreground">
              Project
            </p>
            <p className="mt-1 text-sm font-medium">
              {projectNameById.get(feature.projectId) ?? feature.projectId}
            </p>
          </Link>
        ))}
      </div>
    </PageFrame>
  );
}
