import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/projects/$projectId/coding-plan")({
  component: CodingPlanPage,
});

function CodingPlanPage() {
  return (
    <PlaceholderPage
      description="Coding plan route scaffolded to match the CP-01 target tree."
      title="Coding Plan"
    />
  );
}
