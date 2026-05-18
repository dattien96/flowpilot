import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { supabase } from "@/data/supabase/client";
import { useAuth } from "@/features/auth/auth-provider";
import { hasSupabaseEnv } from "@/lib/env/browser-env";

export const Route = createFileRoute("/login")({
  component: LoginPage,
});

function LoginPage() {
  const navigate = useNavigate();
  const { refreshSession } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const onSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    setErrorMessage(null);

    if (!hasSupabaseEnv()) {
      await refreshSession();
      await navigate({ to: "/dashboard" });
      setSubmitting(false);
      return;
    }

    const { error } = await supabase.auth.signInWithPassword({
      email,
      password,
    });

    if (error) {
      setErrorMessage(error.message);
      setSubmitting(false);
      return;
    }

    await refreshSession();
    await navigate({ to: "/dashboard" });
    setSubmitting(false);
  };

  return (
    <div className="noise-bg flex min-h-screen items-center justify-center px-4 py-10">
      <div className="panel-shadow w-full max-w-md rounded-[2rem] border border-border/80 bg-card/95 p-8 backdrop-blur">
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          FlowPilot
        </p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight">Admin Login</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          Browser-auth foundation for the new Vite admin app.
        </p>

        <form className="mt-8 space-y-4" onSubmit={onSubmit}>
          <label className="block space-y-2">
            <span className="text-sm text-muted-foreground">Email</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3 outline-none"
              disabled={!hasSupabaseEnv() || submitting}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="admin@flowpilot.local"
              type="email"
              value={email}
            />
          </label>
          <label className="block space-y-2">
            <span className="text-sm text-muted-foreground">Password</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3 outline-none"
              disabled={!hasSupabaseEnv() || submitting}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="Password"
              type="password"
              value={password}
            />
          </label>
          {errorMessage ? <p className="text-sm text-danger">{errorMessage}</p> : null}
          <Button className="w-full justify-center" type="submit">
            {hasSupabaseEnv()
              ? submitting
                ? "Signing in..."
                : "Sign in"
              : "Continue in demo mode"}
          </Button>
        </form>
      </div>
    </div>
  );
}
