import { redirect } from "@tanstack/react-router";

import { getBrowserSupabaseClient } from "@/data/supabase/client";
import { getDemoSession, type AdminSession } from "@/features/auth/use-auth";
import { loadSupabaseRuntimeStatus } from "@/lib/supabase/runtime-config";

export async function getOptionalSession(): Promise<AdminSession | null> {
  try {
    const status = await loadSupabaseRuntimeStatus();
    if (!status.configured) {
      return getDemoSession();
    }

    const supabase = await getBrowserSupabaseClient();
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
  } catch (error) {
    console.error("Failed to read the current admin session.", error);
    return null;
  }
}

export async function requireAuth() {
  const session = await getOptionalSession();

  if (!session) {
    throw redirect({ to: "/login" });
  }

  return session;
}
