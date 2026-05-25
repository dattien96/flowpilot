import type { Project } from "@/domain/model/entity/project";
import type { Team, TeamMember } from "@/domain/model/entity/team";
import type { WorkflowRun } from "@/domain/model/entity/workflow";

export interface ProjectDetail {
  project: Project;
  workflowRuns: WorkflowRun[];
  teams: Team[];
  members: TeamMember[];
}
