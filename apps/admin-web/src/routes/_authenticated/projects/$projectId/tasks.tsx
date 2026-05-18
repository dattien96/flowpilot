import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/projects/$projectId/tasks")({
  component: TasksPage,
});

function TasksPage() {
  return (
    <PlaceholderPage
      description="Task management route scaffolded under project detail."
      title="Tasks"
    />
  );
}
