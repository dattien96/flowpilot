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
        "inline-flex items-center justify-center rounded-full px-4 py-2 text-sm font-semibold transition-colors",
        variant === "primary" &&
          "bg-accent text-accent-foreground hover:bg-[#173c2c]",
        variant === "secondary" &&
          "border border-border bg-card text-card-foreground hover:bg-muted",
        variant === "ghost" && "text-muted-foreground hover:bg-muted/80",
        className,
      )}
      type={type}
      {...props}
    />
  );
}
