import { Sun, Moon, Laptop } from "lucide-react";
import { useTheme } from "../use-theme";
import type { ThemePreference } from "../theme-store";
import { cn } from "@/lib/utils/cn";

interface ThemeSwitcherProps {
  className?: string;
  compact?: boolean;
}

export function ThemeSwitcher({ className, compact = false }: ThemeSwitcherProps) {
  const { preference, setPreference } = useTheme();

  const options: Array<{
    value: ThemePreference;
    label: string;
    icon: typeof Sun;
  }> = [
    { value: "light", label: "Light", icon: Sun },
    { value: "dark", label: "Dark", icon: Moon },
    { value: "system", label: "System", icon: Laptop },
  ];

  return (
    <div
      className={cn(
        "grid w-full grid-cols-3 items-stretch gap-1 rounded-[1.25rem] border border-border/80 bg-background/50 p-1 backdrop-blur-sm transition-all",
        className
      )}
    >
      {options.map((opt) => {
        const Icon = opt.icon;
        const active = preference === opt.value;

        return (
          <button
            key={opt.value}
            onClick={() => setPreference(opt.value)}
            className={cn(
              "flex min-w-0 items-center justify-center rounded-[1rem] py-2 transition-all duration-200 outline-none focus-visible:ring-1 focus-visible:ring-accent",
              active
                ? "bg-accent text-accent-foreground shadow-sm"
                : "text-muted-foreground hover:bg-muted/70 hover:text-foreground active:scale-95"
            )}
            title={`Switch to ${opt.label} theme`}
            type="button"
          >
            <Icon className="size-4" />
          </button>
        );
      })}
    </div>
  );
}
