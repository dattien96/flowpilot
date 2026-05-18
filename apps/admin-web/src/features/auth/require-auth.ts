import { redirect } from "@tanstack/react-router";

import { supabase } from "@/data/supabase/client";
import { getDemoSession, type AdminSession } from "@/features/auth/use-auth";
import { hasSupabaseEnv } from "@/lib/env/browser-env";

export async function getOptionalSession(): Promise<AdminSession | null> {
  if (!hasSupabaseEnv()) {
    return getDemoSession();
  }

  const {
    data: { session },
  } = await supabase.auth.getSession();

  if (!session?.user) {
    return null;
  }

  return {
    mode: "supabase",
    user: {
      email: session.user.email ?? null,
      id: session.user.id,
    },
  };
}

export async function requireAuth() {
  const session = await getOptionalSession();

  if (!session) {
    throw redirect({ to: "/login" });
  }

  return session;
}
