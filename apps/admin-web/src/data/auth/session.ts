import { redirect } from "next/navigation";

import { createSupabaseServerClient } from "@/data/datasource/supabase/client";
import { hasSupabaseEnv } from "@/lib/env/app-env";

export interface AdminSession {
  user: {
    id: string;
    email: string | null;
  };
  mode: "supabase" | "demo";
}

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

  const supabase = await createSupabaseServerClient();
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

function readBearerToken(request: Request) {
  const authorization = request.headers.get("authorization")?.trim() ?? "";
  if (!authorization.toLowerCase().startsWith("bearer ")) {
    return "";
  }
  return authorization.slice("bearer ".length).trim();
}

export async function getOptionalAdminSessionForRequest(
  request: Request,
): Promise<AdminSession | null> {
  if (!hasSupabaseEnv()) {
    return {
      mode: "demo",
      user: {
        id: "demo-user",
        email: "demo@flowpilot.local",
      },
    };
  }

  const supabase = await createSupabaseServerClient();
  const bearerToken = readBearerToken(request);
  const { data, error } = bearerToken
    ? await supabase.auth.getUser(bearerToken)
    : await supabase.auth.getUser();

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

export async function requireAdminSession(): Promise<AdminSession> {
  const session = await getOptionalAdminSession();

  if (!session) {
    redirect("/login");
  }

  return session;
}

export async function assertAdminApiSession() {
  const session = await getOptionalAdminSession();

  if (!session) {
    return {
      ok: false as const,
      response: Response.json({ error: "Authentication required." }, { status: 401 }),
    };
  }

  return {
    ok: true as const,
    session,
  };
}

export async function assertAdminApiSessionForRequest(request: Request) {
  const session = await getOptionalAdminSessionForRequest(request);

  if (!session) {
    return {
      ok: false as const,
      response: Response.json({ error: "Authentication required." }, { status: 401 }),
    };
  }

  return {
    ok: true as const,
    session,
  };
}
