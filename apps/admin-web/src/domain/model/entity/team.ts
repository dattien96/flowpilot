export type MemberRole =
  | "android"
  | "ios"
  | "backend"
  | "frontend"
  | "qa"
  | "devops"
  | "ai_workflow";

export type LevelLabel =
  | "L1_intern"
  | "L2_junior"
  | "L3_middle"
  | "L4_senior"
  | "L5_lead";

export interface Team {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
}

export interface TeamMember {
  id: string;
  teamId: string;
  name: string;
  email: string | null;
  jiraAccountId: string | null;
  role: MemberRole;
  levelLabel: LevelLabel;
  skillTags: string[];
  weeklyCapacityHours: number;
  createdAt: string;
  updatedAt: string;
}
