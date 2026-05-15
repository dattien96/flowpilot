import { ArrowRight } from "lucide-react";

import { Button } from "@/presentation/components/ui/button";

export default function LoginPage() {
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
          The real Supabase auth wiring comes next. For the current skeleton,
          the admin runs in demo mode and focuses on workflow state, approvals,
          outputs, and logs.
        </p>
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
      </div>
    </div>
  );
}
