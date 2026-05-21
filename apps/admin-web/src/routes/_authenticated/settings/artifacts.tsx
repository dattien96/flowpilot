import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/_authenticated/settings/artifacts")({
  beforeLoad: () => {
    throw redirect({ to: "/artifacts" });
  },
});
