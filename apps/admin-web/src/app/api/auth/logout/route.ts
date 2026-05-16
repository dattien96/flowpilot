import { redirect } from "next/navigation";
import { NextResponse } from "next/server";

import { createSupabaseServerClient } from "@/data/datasource/supabase/client";
import { hasSupabaseEnv } from "@/lib/env/app-env";

export async function POST(request: Request) {
  if (hasSupabaseEnv()) {
    const supabase = await createSupabaseServerClient();
    await supabase.auth.signOut();
  }

  if ((request.headers.get("content-type") ?? "").includes("application/json")) {
    return NextResponse.json({ ok: true });
  }

  redirect("/login");
}
