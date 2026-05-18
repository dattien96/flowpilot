import { createFileRoute } from "@tanstack/react-router";

import { PlaceholderPage } from "@/components/common/placeholder-page";

export const Route = createFileRoute("/_authenticated/settings/prompt-templates")({
  component: PromptTemplatesPage,
});

function PromptTemplatesPage() {
  return (
    <PlaceholderPage
      description="Prompt template settings route scaffolded for later feature work."
      title="Prompt Templates"
    />
  );
}
