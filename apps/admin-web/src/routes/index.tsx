import { Navigate, createFileRoute } from "@tanstack/react-router";

import { useAuth } from "@/features/auth/auth-provider";

export const Route = createFileRoute("/")({
  component: HomePage,
});

function HomePage() {
  const { loading, session } = useAuth();

  if (loading) {
    return (
      <div className="noise-bg flex min-h-screen items-center justify-center px-4 py-10">
        <div className="panel-shadow w-full max-w-md rounded-[2rem] border border-border/80 bg-card/95 p-8 text-center backdrop-blur">
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            FlowPilot
          </p>
          <h1 className="mt-3 text-3xl font-semibold tracking-tight">
            Preparing your workspace
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Checking your admin session before redirecting.
          </p>
        </div>
      </div>
    );
  }

  return <Navigate replace to={session ? "/dashboard" : "/login"} />;
}
