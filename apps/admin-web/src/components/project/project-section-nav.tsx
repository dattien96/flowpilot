import { Link, useLocation } from "@tanstack/react-router";

import { cn } from "@/lib/utils/cn";

const tabs = [
  { label: "Overview", suffix: "" },
  { label: "Business Logic", suffix: "/business-logic" },
  { label: "Tech Specs", suffix: "/tech-specs" },
  { label: "Coding Plan", suffix: "/coding-plan" },
  { label: "Master Schedule", suffix: "/master-schedule" },
  { label: "Tasks", suffix: "/tasks" },
  { label: "Members", suffix: "/members" },
  { label: "Workflows", suffix: "/workflows" },
  { label: "Artifacts", suffix: "/artifacts" },
  { label: "Settings", suffix: "/settings" },
];

export function ProjectSectionNav({ projectId }: { projectId: string }) {
  const location = useLocation();

  return (
    <div className="flex flex-wrap gap-2">
      {tabs.map((tab) => {
        const to = `/projects/${projectId}${tab.suffix}`;
        const active = tab.suffix === ""
          ? location.pathname === `/projects/${projectId}`
          : location.pathname === to;

        return (
          <Link
            key={tab.suffix}
            className={cn(
              "rounded-full px-4 py-2 text-sm transition-colors",
              active
                ? "bg-accent text-accent-foreground"
                : "bg-muted text-muted-foreground hover:text-foreground",
            )}
            to={to}
          >
            {tab.label}
          </Link>
        );
      })}
    </div>
  );
}
