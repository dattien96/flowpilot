import type { ReactNode } from "react";

export function PageFrame({
  actions,
  children,
  description,
  title,
}: {
  actions?: ReactNode;
  children?: ReactNode;
  description: string;
  title: string;
}) {
  return (
    <section className="space-y-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div className="space-y-2">
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            CP-01 Foundation
          </p>
          <h2 className="text-3xl font-semibold tracking-tight">{title}</h2>
          <p className="max-w-2xl text-sm text-muted-foreground">{description}</p>
        </div>
        {actions}
      </div>
      {children}
    </section>
  );
}
