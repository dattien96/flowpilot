import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { GenerationLaunchPanel } from "@/features/ai-orchestration/generation-launch-panel";

export const Route = createFileRoute("/_authenticated/projects/$projectId/master-schedule")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const bindings = await gateways.projectGateway.listProjectWorkspaceBindings(params.projectId);
    return { bindings, projectId: params.projectId };
  },
  component: ProjectMasterSchedulePage,
});

function ProjectMasterSchedulePage() {
  const { bindings, projectId } = Route.useLoaderData();

  return (
    <PageFrame description="Master schedule generation for project management." title="Master Schedule">
      <ProjectSectionNav projectId={projectId} />
      <div className="mt-6">
        <GenerationLaunchPanel
          buttonLabel="Generate Schedule"
          description="Convert the coding plan into milestone slices and delivery checkpoints for the project schedule."
          promptHint="Include implementation order, checkpoints, and any delivery constraints that should shape the schedule."
          promptLabel="Coding plan source"
          promptPlaceholder="Paste the coding plan or the delivery assumptions that should be planned."
          bindings={bindings}
          projectId={projectId}
          stepType="task_breakdown"
          title="Generate master schedule"
        />
      </div>
    </PageFrame>
  );
}
