import type { LevelLabel, MemberRole, TeamMember } from "@/domain/model/entity/team";

export interface CreateTeamPayload {
  name: string;
}

export interface UpdateTeamPayload {
  teamId: string;
  name: string;
}

export interface AddTeamMemberPayload {
  teamId: string;
  name: string;
  email?: string | null;
  jiraAccountId?: string | null;
  role: MemberRole;
  levelLabel: LevelLabel;
  skillTags?: string[];
  weeklyCapacityHours?: number;
}

export interface UpdateTeamMemberPayload {
  memberId: string;
  patch: Partial<Omit<TeamMember, "id" | "teamId" | "createdAt" | "updatedAt">>;
}
