import { redirect } from "next/navigation";
import { NextResponse } from "next/server";

import { createSupabaseServerClient } from "@/data/datasource/supabase/client";
import { hasSupabaseRuntimeConfigOrEnvFallback } from "@/lib/supabase/runtime-config.server";

export async function POST(request: Request) {
  if (await hasSupabaseRuntimeConfigOrEnvFallback()) {
    const supabase = await createSupabaseServerClient();
    await supabase.auth.signOut();
  }

  if ((request.headers.get("content-type") ?? "").includes("application/json")) {
    return NextResponse.json({ ok: true });
  }

  redirect("/login");
}
