import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { GenerationLaunchPanel } from "@/features/ai-orchestration/generation-launch-panel";

export const Route = createFileRoute("/_authenticated/projects/$projectId/coding-plan")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const bindings = await gateways.projectGateway.listProjectWorkspaceBindings(params.projectId);
    return { bindings, projectId: params.projectId };
  },
  component: ProjectCodingPlanPage,
});

function ProjectCodingPlanPage() {
  const { bindings, projectId } = Route.useLoaderData();

  return (
    <PageFrame description="Coding plan generation for project management." title="Coding Plan">
      <ProjectSectionNav projectId={projectId} />
      <div className="mt-6">
        <GenerationLaunchPanel
          buttonLabel="Generate from Tech Spec"
          description="Translate the technical specification into a concrete implementation plan that can drive the code-review loop."
          promptHint="Include architecture constraints, implementation boundaries, and any known risks."
          promptLabel="Tech spec source"
          promptPlaceholder="Paste the technical specification or the important implementation constraints."
          bindings={bindings}
          projectId={projectId}
          stepType="make_plan_coding"
          title="Generate coding plan"
        />
      </div>
    </PageFrame>
  );
}
