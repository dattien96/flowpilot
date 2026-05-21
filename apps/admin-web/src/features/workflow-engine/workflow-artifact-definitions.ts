export interface WorkflowArtifactDefinitionOption {
  key: string;
  label: string;
  defaultFileName: string;
}

export const workflowArtifactDefinitionOptions: WorkflowArtifactDefinitionOption[] = [
  { key: "business_idea_artifact", label: "Business Idea", defaultFileName: "BusinessIdea.md" },
  {
    key: "feature_intake_artifact",
    label: "Feature Intake",
    defaultFileName: "FeatureIntake.md",
  },
  {
    key: "business_summary_artifact",
    label: "Business Summary",
    defaultFileName: "BusinessSummary.md",
  },
  { key: "product_spec_artifact", label: "Product Spec", defaultFileName: "ProductSpec.md" },
  { key: "tech_spec_artifact", label: "Tech Spec", defaultFileName: "TechSpec.md" },
  { key: "coding_plan_artifact", label: "Coding Plan", defaultFileName: "CodingPlan.md" },
  {
    key: "coding_implementation_checklist_artifact",
    label: "Coding Implementation Checklist",
    defaultFileName: "CodingImplementationChecklist.md",
  },
  {
    key: "architecture_artifact",
    label: "Architecture",
    defaultFileName: "ArchitecturePlan.md",
  },
  { key: "tdd_plan_artifact", label: "TDD Plan", defaultFileName: "TddPlan.md" },
  {
    key: "task_breakdown_artifact",
    label: "Task Breakdown",
    defaultFileName: "TaskBreakdown.md",
  },
  {
    key: "code_review_summary_artifact",
    label: "Code Review Summary",
    defaultFileName: "CodeReviewSummary.md",
  },
  {
    key: "release_readiness_artifact",
    label: "Release Readiness",
    defaultFileName: "ReleaseReadiness.md",
  },
  {
    key: "root_cause_analysis_artifact",
    label: "Root Cause Analysis",
    defaultFileName: "RootCauseAnalysis.md",
  },
  {
    key: "usage_analytics_artifact",
    label: "Usage Analytics",
    defaultFileName: "UsageAnalytics.md",
  },
  {
    key: "project_analysis_artifact",
    label: "Project Analysis",
    defaultFileName: "ProjectAnalysis.md",
  },
  {
    key: "code_traceability_artifact",
    label: "Code Traceability",
    defaultFileName: "CodeTraceability.md",
  },
];

export function firstAvailableWorkflowArtifactDefinition(selectedKeys: string[]) {
  return workflowArtifactDefinitionOptions.find((option) => !selectedKeys.includes(option.key))?.key ?? "";
}

export function getWorkflowArtifactDefinitionOption(key: string) {
  return workflowArtifactDefinitionOptions.find((option) => option.key === key) ?? null;
}
