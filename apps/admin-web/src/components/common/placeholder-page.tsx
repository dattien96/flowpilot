import { Link } from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
import { PageFrame } from "@/components/common/page-frame";

export function PlaceholderPage({
  ctaLabel,
  ctaTo,
  description,
  title,
}: {
  ctaLabel?: string;
  ctaTo?: string;
  description: string;
  title: string;
}) {
  return (
    <PageFrame
      actions={
        ctaLabel && ctaTo ? (
          <Link to={ctaTo}>
            <Button>{ctaLabel}</Button>
          </Link>
        ) : null
      }
      description={description}
      title={title}
    >
      <div className="rounded-[1.75rem] border border-dashed border-border bg-background/60 p-6">
        <p className="text-sm text-muted-foreground">
          This route is scaffolded and wired into the foundation router. Feature-level
          implementation will land in later coding plans.
        </p>
      </div>
    </PageFrame>
  );
}
