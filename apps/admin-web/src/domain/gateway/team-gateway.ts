import type { Team, TeamMember } from "@/domain/model/entity/team";

export interface TeamGateway {
  listTeams(): Promise<Team[]>;
  getTeamById(teamId: string): Promise<Team | null>;
  createTeam(name: string): Promise<Team>;
  updateTeam(teamId: string, name: string): Promise<Team>;
  deleteTeam(teamId: string): Promise<void>;
  listMembersByTeam(teamId: string): Promise<TeamMember[]>;
  addMember(member: Omit<TeamMember, "id" | "createdAt" | "updatedAt">): Promise<TeamMember>;
  updateMember(memberId: string, patch: Partial<TeamMember>): Promise<TeamMember>;
  removeMember(memberId: string): Promise<void>;
  linkTeamToProject(projectId: string, teamId: string): Promise<void>;
  unlinkTeamFromProject(projectId: string, teamId: string): Promise<void>;
  listTeamsByProject(projectId: string): Promise<Team[]>;
}
