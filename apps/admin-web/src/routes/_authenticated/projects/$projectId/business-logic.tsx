import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";

export const Route = createFileRoute("/_authenticated/projects/$projectId/business-logic")({
  component: ProjectBusinessLogicPage,
});

function ProjectBusinessLogicPage() {
  const { projectId } = Route.useParams();

  return (
    <PageFrame description="Business logic placeholder for project management." title="Business Logic">
      <ProjectSectionNav projectId={projectId} />
      <p className="mt-6 text-sm text-muted-foreground">
        This tab is reserved for future business-rule detail.
      </p>
    </PageFrame>
  );
}
