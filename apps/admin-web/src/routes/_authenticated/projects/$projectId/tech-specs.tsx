import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/projects/$projectId/tech-specs")({
  component: TechSpecsPage,
});

function TechSpecsPage() {
  return (
    <PlaceholderPage
      description="Technical specification route scaffolded under the new project layout."
      title="Tech Specs"
    />
  );
}
