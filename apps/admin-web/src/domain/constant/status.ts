export type WorkflowRunStatus =
  | "draft"
  | "running"
  | "waiting_approval"
  | "completed"
  | "rejected"
  | "failed";

export type WorkflowStepStatus =
  | "pending"
  | "running"
  | "waiting_approval"
  | "completed"
  | "rejected"
  | "failed";

export type ApprovalStatus =
  | "pending"
  | "approved"
  | "rejected"
  | "changes_requested";

export type OutputType =
  | "business_summary"
  | "product_spec"
  | "android_tech_spec"
  | "task_breakdown"
  | "test_plan"
  | "risk_report";
