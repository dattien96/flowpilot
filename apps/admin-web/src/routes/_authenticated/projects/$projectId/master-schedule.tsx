import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";

export const Route = createFileRoute("/_authenticated/projects/$projectId/master-schedule")({
  component: ProjectMasterSchedulePage,
});

function ProjectMasterSchedulePage() {
  const { projectId } = Route.useParams();

  return (
    <PageFrame description="Master schedule placeholder for project management." title="Master Schedule">
      <ProjectSectionNav projectId={projectId} />
      <p className="mt-6 text-sm text-muted-foreground">
        Delivery timeline and milestone planning will live here.
      </p>
    </PageFrame>
  );
}
