import type { Feature } from "@/domain/model/entity/feature";
import type { Project } from "@/domain/model/entity/project";
import type { WorkflowRun } from "@/domain/model/entity/workflow";

export interface ProjectDetail {
  project: Project;
  features: Feature[];
  workflowRuns: WorkflowRun[];
}
