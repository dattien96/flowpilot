import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/projects/$projectId/settings")({
  component: ProjectSettingsPage,
});

function ProjectSettingsPage() {
  return (
    <PlaceholderPage
      description="Project settings route scaffolded under project detail."
      title="Project Settings"
    />
  );
}
