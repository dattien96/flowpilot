import type { ReactNode } from "react";

interface StatCardProps {
  label: string;
  value: number | string;
  hint: string;
  accent?: ReactNode;
}

export function StatCard({ label, value, hint, accent }: StatCardProps) {
  return (
    <section className="rounded-[1.6rem] border border-border bg-background/70 p-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="font-mono text-[11px] uppercase tracking-[0.28em] text-muted-foreground">
            {label}
          </p>
          <p className="mt-4 text-4xl font-semibold tracking-tight">{value}</p>
        </div>
        {accent}
      </div>
      <p className="mt-5 text-sm text-muted-foreground">{hint}</p>
    </section>
  );
}
