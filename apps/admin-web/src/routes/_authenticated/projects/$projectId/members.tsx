import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/projects/$projectId/members")({
  component: MembersPage,
});

function MembersPage() {
  return (
    <PlaceholderPage
      description="Members route scaffolded under project detail."
      title="Members"
    />
  );
}
