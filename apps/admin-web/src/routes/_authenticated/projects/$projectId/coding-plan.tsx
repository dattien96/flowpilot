import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";

export const Route = createFileRoute("/_authenticated/projects/$projectId/coding-plan")({
  component: ProjectCodingPlanPage,
});

function ProjectCodingPlanPage() {
  const { projectId } = Route.useParams();

  return (
    <PageFrame description="Coding plan placeholder for project management." title="Coding Plan">
      <ProjectSectionNav projectId={projectId} />
      <p className="mt-6 text-sm text-muted-foreground">
        Implementation slices for this project will be tracked here.
      </p>
    </PageFrame>
  );
}
