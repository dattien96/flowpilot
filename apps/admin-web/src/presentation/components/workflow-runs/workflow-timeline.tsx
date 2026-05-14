import type { WorkflowStep } from "@/domain/model/entity/workflow";
import { Badge } from "@/presentation/components/ui/badge";

interface WorkflowTimelineProps {
  steps: WorkflowStep[];
}

export function WorkflowTimeline({ steps }: WorkflowTimelineProps) {
  return (
    <div className="space-y-3">
      {steps.map((step) => (
        <div
          key={step.id}
          className="flex items-center justify-between rounded-2xl border border-border bg-background/70 px-4 py-3"
        >
          <div>
            <p className="font-semibold">{step.stepName}</p>
            <p className="text-sm text-muted-foreground">{step.stepKey}</p>
          </div>
          <Badge
            tone={
              step.status === "completed"
                ? "success"
                : step.status === "waiting_approval"
                  ? "warning"
                  : step.status === "rejected" || step.status === "failed"
                    ? "danger"
                    : "neutral"
            }
          >
            {step.status}
          </Badge>
        </div>
      ))}
    </div>
  );
}
