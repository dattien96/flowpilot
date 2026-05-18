import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/projects/$projectId/workflows")({
  component: WorkflowsPage,
});

function WorkflowsPage() {
  return (
    <PlaceholderPage
      description="Workflows route scaffolded under project detail."
      title="Workflows"
    />
  );
}
