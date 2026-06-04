import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { getBrowserSupabaseClient } from "@/data/supabase/client";
import { useAuth } from "@/features/auth/auth-provider";
import {
  loadSupabaseRuntimeStatus,
  type SupabaseRuntimeStatus,
} from "@/lib/supabase/runtime-config";

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
  const [runtimeStatus, setRuntimeStatus] = useState<SupabaseRuntimeStatus | null>(null);

  useEffect(() => {
    let active = true;
    void loadSupabaseRuntimeStatus().then((status) => {
      if (active) {
        setRuntimeStatus(status);
      }
    });
    return () => {
      active = false;
    };
  }, []);

  const configured = runtimeStatus?.configured === true;

  const onSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitting) {
      return;
    }

    setSubmitting(true);
    setErrorMessage(null);

    try {
      const status = runtimeStatus ?? (await loadSupabaseRuntimeStatus());
      if (!status.configured) {
        setErrorMessage("Supabase database is not configured. Please configure this PC first.");
        return;
      }

      const supabase = await getBrowserSupabaseClient();
      const { error } = await supabase.auth.signInWithPassword({
        email,
        password,
      });

      if (error) {
        setErrorMessage(error.message);
        return;
      }

      await refreshSession();
      void navigate({ to: "/dashboard" });
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "Unable to sign in.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="noise-bg flex min-h-screen items-center justify-center px-4 py-10">
      <div className="panel-shadow w-full max-w-md rounded-[2rem] border border-border/80 bg-card/95 p-8 backdrop-blur">
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          FlowPilot
        </p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight">Admin Login</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          Sign in to the active workspace Supabase project or configure this PC first.
        </p>

        {!runtimeStatus ? (
          <div className="mt-8 rounded-2xl border border-border bg-background/70 px-4 py-3 text-sm text-muted-foreground">
            Checking database configuration...
          </div>
        ) : configured ? (
          <form className="mt-8 space-y-4" onSubmit={onSubmit}>
            <label className="block space-y-2">
              <span className="text-sm text-muted-foreground">Email</span>
              <input
                className="w-full rounded-2xl border border-border bg-background px-4 py-3 outline-none transition-all focus:border-transparent focus:ring-2 focus:ring-accent"
                disabled={submitting}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="admin@flowpilot.local"
                type="email"
                value={email}
              />
            </label>
            <label className="block space-y-2">
              <span className="text-sm text-muted-foreground">Password</span>
              <input
                className="w-full rounded-2xl border border-border bg-background px-4 py-3 outline-none transition-all focus:border-transparent focus:ring-2 focus:ring-accent"
                disabled={submitting}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="Password"
                type="password"
                value={password}
              />
            </label>
            {errorMessage ? <p className="text-sm text-danger">{errorMessage}</p> : null}
            <Button className="w-full justify-center" type="submit">
              {submitting ? "Signing in..." : "Sign in"}
            </Button>
            <Link className="block text-center text-sm text-muted-foreground hover:text-foreground" to="/setup/supabase">
              Database Settings
            </Link>
          </form>
        ) : (
          <div className="mt-8 space-y-4">
            <div className="rounded-2xl border border-warning/30 bg-warning/10 px-4 py-3 text-sm text-warning">
              Supabase database is not configured. Realtime features, persistence, and team controls are unavailable in Demo Mode.
              {runtimeStatus.lastError ? (
                <span className="mt-2 block text-xs">{runtimeStatus.lastError}</span>
              ) : null}
            </div>
            <Link to="/setup/supabase">
              <Button className="w-full justify-center" type="button">
                Configure Supabase
              </Button>
            </Link>
          </div>
        )}
      </div>
    </div>
  );
}
