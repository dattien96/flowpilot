import type { ReactNode } from "react";

import { cn } from "@/lib/utils/cn";

interface BadgeProps {
  tone?: "neutral" | "success" | "warning" | "danger";
  children: ReactNode;
}

export function Badge({ tone = "neutral", children }: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex rounded-full px-2.5 py-1 text-xs font-medium uppercase tracking-[0.18em]",
        tone === "neutral" && "bg-muted text-muted-foreground",
        tone === "success" && "bg-success/12 text-success",
        tone === "warning" && "bg-warning/14 text-warning",
        tone === "danger" && "bg-danger/12 text-danger",
      )}
    >
      {children}
    </span>
  );
}
