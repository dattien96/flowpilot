import { Outlet, createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";

export const Route = createFileRoute("/_authenticated/projects/$projectId")({
  component: ProjectLayout,
});

function ProjectLayout() {
  const { projectId } = Route.useParams();

  return (
    <PageFrame
      description="Each project route nests under a dedicated layout so later feature phases can fill in section-specific data and tools."
      title={`Project ${projectId}`}
    >
      <ProjectSectionNav projectId={projectId} />
      <Outlet />
    </PageFrame>
  );
}
