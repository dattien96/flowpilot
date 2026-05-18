import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/settings/integrations")({
  component: IntegrationsPage,
});

function IntegrationsPage() {
  return (
    <PlaceholderPage
      description="Integration settings route scaffolded for browser-first configuration work."
      title="Integrations"
    />
  );
}
