import { ArrowRight } from "lucide-react";

import { hasSupabaseEnv } from "@/lib/env/app-env";
import { Button } from "@/presentation/components/ui/button";

export default function LoginPage() {
  const supabaseEnabled = hasSupabaseEnv();

  return (
    <div className="noise-bg flex min-h-screen items-center justify-center px-4">
      <div className="panel-shadow w-full max-w-xl rounded-[2rem] border border-border bg-card p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-muted-foreground">
          FlowPilot Access
        </p>
        <h1 className="mt-4 text-4xl font-semibold tracking-tight">
          Enter the workflow control center.
        </h1>
        <p className="mt-4 max-w-lg text-base text-muted-foreground">
          {supabaseEnabled
            ? "Sign in with a Supabase Auth user to access protected admin workflows."
            : "Supabase env is not configured, so the admin runs in demo mode for local exploration."}
        </p>
        {supabaseEnabled ? (
          <form action="/api/auth/login" method="post" className="mt-8 grid gap-3">
            <input
              className="rounded-2xl border border-border bg-background px-4 py-3"
              name="email"
              placeholder="admin@example.com"
              required
              type="email"
            />
            <input
              className="rounded-2xl border border-border bg-background px-4 py-3"
              name="password"
              placeholder="Password"
              required
              type="password"
            />
            <Button type="submit">Sign in</Button>
          </form>
        ) : (
          <div className="mt-8 flex gap-3">
          <a className="inline-flex" href="/dashboard">
            <Button>
              <span className="inline-flex items-center gap-2">
                Continue to demo
                <ArrowRight className="size-4" />
              </span>
            </Button>
          </a>
          </div>
        )}
      </div>
    </div>
  );
}
