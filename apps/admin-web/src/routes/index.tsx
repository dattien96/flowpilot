import { createFileRoute, redirect } from "@tanstack/react-router";

import { getOptionalSession } from "@/features/auth/require-auth";

export const Route = createFileRoute("/")({
  beforeLoad: async () => {
    const session = await getOptionalSession();

    throw redirect({
      to: session ? "/dashboard" : "/login",
    });
  },
});
