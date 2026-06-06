import type { ButtonHTMLAttributes } from "react";

import { cn } from "@/lib/utils/cn";

type ButtonVariant = "primary" | "secondary" | "ghost";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
}

export function Button({
  className,
  variant = "primary",
  type = "button",
  ...props
}: ButtonProps) {
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center rounded-full px-4 py-2 text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50",
        variant === "primary" &&
          "bg-accent text-accent-foreground hover:bg-accent/90 active:bg-accent/80",
        variant === "secondary" &&
          "border border-border bg-card text-card-foreground hover:bg-muted active:bg-muted/80",
        variant === "ghost" && "text-muted-foreground hover:bg-muted/80 hover:text-foreground",
        className,
      )}
      type={type}
      {...props}
    />
  );
}
