import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";

export const Route = createFileRoute("/_authenticated/projects/$projectId/tech-specs")({
  component: ProjectTechSpecsPage,
});

function ProjectTechSpecsPage() {
  const { projectId } = Route.useParams();

  return (
    <PageFrame description="Tech specs placeholder for project management." title="Tech Specs">
      <ProjectSectionNav projectId={projectId} />
      <p className="mt-6 text-sm text-muted-foreground">
        Architecture and platform constraints will be tracked here in a later phase.
      </p>
    </PageFrame>
  );
}
