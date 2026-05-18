import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";

export const Route = createFileRoute("/_authenticated/projects/$projectId/workflows")({
  component: ProjectWorkflowsPage,
});

function ProjectWorkflowsPage() {
  const { projectId } = Route.useParams();

  return (
    <PageFrame description="Workflows placeholder for project management." title="Workflows">
      <ProjectSectionNav projectId={projectId} />
      <p className="mt-6 text-sm text-muted-foreground">
        Workflow orchestration details remain on the existing workflow pages.
      </p>
    </PageFrame>
  );
}
