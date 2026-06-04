import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/_authenticated/settings/mcp-servers/instances")({
  beforeLoad: () => {
    throw redirect({ to: "/settings/mcp-servers" });
  },
  component: () => null,
});
