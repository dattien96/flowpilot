import type { WorkflowRunStatus } from "@/domain/constant/status";

export function statusTone(status: WorkflowRunStatus) {
  if (status === "completed") return "success";
  if (status === "waiting_approval") return "warning";
  if (status === "rejected" || status === "failed") return "danger";
  return "neutral";
}
