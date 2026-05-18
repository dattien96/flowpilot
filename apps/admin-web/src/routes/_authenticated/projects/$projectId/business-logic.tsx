import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/projects/$projectId/business-logic")({
  component: BusinessLogicPage,
});

function BusinessLogicPage() {
  return (
    <PlaceholderPage
      description="Business logic planning surface is routed and ready for later feature implementation."
      title="Business Logic"
    />
  );
}
