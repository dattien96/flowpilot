import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { DashboardFavorites } from "@/components/dashboard/dashboard-favorites";

export const Route = createFileRoute("/_authenticated/dashboard")({
  component: DashboardPage,
});

function DashboardPage() {
  return (
    <>
      <PageFrame
        actions={
          <div className="flex items-center gap-2">
            <Button variant="outline" className="rounded-xl">Foundation Ready</Button>
          </div>
        }
        description="The shell, router, and auth guard are active. This dashboard acts as the authenticated landing page for CP-01."
      title="Dashboard"
    >
      <div className="grid gap-4 md:grid-cols-3">
        {[
          "Vite app shell established",
          "TanStack Router file routes active",
          "Supabase browser auth wired",
        ].map((item) => (
          <div key={item} className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <p className="text-sm text-muted-foreground">{item}</p>
          </div>
        ))}
      </div>
      <DashboardFavorites />
      </PageFrame>
    </>
  );
}
