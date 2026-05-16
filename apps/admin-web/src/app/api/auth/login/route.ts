import { redirect } from "next/navigation";
import { NextResponse } from "next/server";
import { z } from "zod";

import { createSupabaseServerClient } from "@/data/datasource/supabase/client";
import { hasSupabaseEnv } from "@/lib/env/app-env";

const loginSchema = z.object({
  email: z.string().email(),
  password: z.string().min(1),
});

export async function POST(request: Request) {
  if (!hasSupabaseEnv()) {
    redirect("/dashboard");
  }

  const contentType = request.headers.get("content-type") ?? "";
  const input = contentType.includes("application/json")
    ? await request.json()
    : Object.fromEntries((await request.formData()).entries());
  const payload = loginSchema.parse(input);
  const supabase = await createSupabaseServerClient();
  const { error } = await supabase.auth.signInWithPassword(payload);

  if (error) {
    if (contentType.includes("application/json")) {
      return NextResponse.json({ error: error.message }, { status: 401 });
    }

    redirect("/login?error=invalid_credentials");
  }

  if (contentType.includes("application/json")) {
    return NextResponse.json({ ok: true });
  }

  redirect("/dashboard");
}
