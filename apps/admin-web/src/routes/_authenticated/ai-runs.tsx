import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/ai-runs")({
  component: AiRunsPage,
});

function AiRunsPage() {
  return (
    <PlaceholderPage
      description="AI run monitoring route scaffolded as part of the foundation route tree."
      title="AI Runs"
    />
  );
}
