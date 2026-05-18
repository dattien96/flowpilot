import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/projects/$projectId/master-schedule")({
  component: MasterSchedulePage,
});

function MasterSchedulePage() {
  return (
    <PlaceholderPage
      description="Master schedule route scaffolded under project detail."
      title="Master Schedule"
    />
  );
}
