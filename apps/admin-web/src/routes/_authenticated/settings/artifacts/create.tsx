import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/_authenticated/settings/artifacts/create")({
  beforeLoad: () => {
    throw redirect({ to: "/artifacts/create" });
  },
});
