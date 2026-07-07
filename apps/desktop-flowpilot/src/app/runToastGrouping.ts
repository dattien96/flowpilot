export type RunToastKind = "done" | "approval" | "question" | "blocked";

export interface RunToastGroupItem {
  kind: RunToastKind;
}

export const COLLAPSIBLE_TOAST_THRESHOLD = 1;

const KIND_LABELS: Record<RunToastKind, string> = {
  done: "Completed",
  approval: "Approvals",
  question: "Questions",
  blocked: "Paused",
};

export function shouldCollapseToasts(count: number): boolean {
  return count >= COLLAPSIBLE_TOAST_THRESHOLD;
}

export function buildToastGroupSummary(toasts: readonly RunToastGroupItem[]): string {
  const counts: Record<RunToastKind, number> = {
    done: 0,
    approval: 0,
    question: 0,
    blocked: 0,
  };

  for (const toast of toasts) {
    counts[toast.kind] += 1;
  }

  return (Object.keys(counts) as RunToastKind[])
    .filter((kind) => counts[kind] > 0)
    .map((kind) => `${KIND_LABELS[kind]} ${counts[kind]}`)
    .join(" · ");
}
