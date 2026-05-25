import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { GenerationLaunchPanel } from "@/features/ai-orchestration/generation-launch-panel";

export const Route = createFileRoute("/_authenticated/projects/$projectId/business-logic")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const bindings = await gateways.projectGateway.listProjectWorkspaceBindings(params.projectId);
    return { bindings, projectId: params.projectId };
  },
  component: ProjectBusinessLogicPage,
});

function ProjectBusinessLogicPage() {
  const { bindings, projectId } = Route.useLoaderData();

  return (
    <PageFrame description="Business logic review and generation for project management." title="Business Logic">
      <ProjectSectionNav projectId={projectId} />
      <div className="mt-6">
        <GenerationLaunchPanel
          buttonLabel="Ask AI to Review"
          description="Launch the canonical review step for the current business-logic draft. The created run keeps workflow lineage and artifact output consistent with the rest of the engine."
          promptHint="Describe the business logic, key constraints, or unresolved decisions that should be reviewed."
          promptLabel="Business logic notes"
          promptPlaceholder="Summarize the current business rules, edge cases, and open questions."
          bindings={bindings}
          projectId={projectId}
          stepType="business_summary"
          title="Review business logic"
          tone="warning"
        />
      </div>
    </PageFrame>
  );
}
