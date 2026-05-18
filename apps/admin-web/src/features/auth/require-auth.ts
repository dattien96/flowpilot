import { redirect } from "@tanstack/react-router";

import { hasSupabaseEnv } from "@/lib/env/browser-env";
import type { AdminSession } from "@/features/auth/auth-provider";
import { createSupabaseBrowserClient } from "@/data/supabase/client";

export async function getOptionalAdminSession(): Promise<AdminSession | null> {
  if (!hasSupabaseEnv()) {
    return {
      mode: "demo",
      user: {
        id: "demo-user",
        email: "demo@flowpilot.local",
      },
    };
  }

  const supabase = createSupabaseBrowserClient();
  const { data, error } = await supabase.auth.getUser();

  if (error || !data.user) {
    return null;
  }

  return {
    mode: "supabase",
    user: {
      id: data.user.id,
      email: data.user.email ?? null,
    },
  };
}

export async function requireAdminSession() {
  const session = await getOptionalAdminSession();

  if (!session) {
    throw redirect({ to: "/login" });
  }

  return session;
}

