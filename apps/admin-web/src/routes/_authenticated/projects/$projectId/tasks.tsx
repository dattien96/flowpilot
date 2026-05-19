import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";

export const Route = createFileRoute("/_authenticated/projects/$projectId/tasks")({
  component: ProjectTasksPage,
});

function ProjectTasksPage() {
  const { projectId } = Route.useParams();

  return (
    <PageFrame description="Tasks placeholder for project management." title="Tasks">
      <ProjectSectionNav projectId={projectId} />
      <p className="mt-6 text-sm text-muted-foreground">
        Task tracking will be connected in a later phase.
      </p>
    </PageFrame>
  );
}
