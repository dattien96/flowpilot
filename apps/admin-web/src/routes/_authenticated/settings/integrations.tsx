import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/settings/integrations")({
  component: IntegrationsPage,
});

function IntegrationsPage() {
  return (
    <PlaceholderPage
      description="Project-scoped MCP connections live under Project > Settings. Use this page only as a pointer back to the project settings surface."
      title="Integrations"
    />
  );
}
