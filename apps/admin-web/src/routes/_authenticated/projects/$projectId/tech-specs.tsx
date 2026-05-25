import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { GenerationLaunchPanel } from "@/features/ai-orchestration/generation-launch-panel";

export const Route = createFileRoute("/_authenticated/projects/$projectId/tech-specs")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const bindings = await gateways.projectGateway.listProjectWorkspaceBindings(params.projectId);
    return { bindings, projectId: params.projectId };
  },
  component: ProjectTechSpecsPage,
});

function ProjectTechSpecsPage() {
  const { bindings, projectId } = Route.useLoaderData();

  return (
    <PageFrame description="Tech spec generation for project management." title="Tech Specs">
      <ProjectSectionNav projectId={projectId} />
      <div className="mt-6">
        <GenerationLaunchPanel
          buttonLabel="Generate from Business Logic"
          description="Turn the business-logic draft into a canonical technical specification using the workflow engine."
          promptHint="Include the business requirements, assumptions, and platform constraints that the tech spec should respect."
          promptLabel="Business logic source"
          promptPlaceholder="Paste the business logic summary or the key decisions that should shape the tech spec."
          bindings={bindings}
          projectId={projectId}
          stepType="tech_spec"
          title="Generate tech spec"
        />
      </div>
    </PageFrame>
  );
}
