import type { Team } from "@/domain/model/entity/team";

export function getTeamLinkDelta(currentTeams: Team[], nextTeamIds: string[]) {
  const normalizedNextIds = Array.from(
    new Set(nextTeamIds.map((teamId) => teamId.trim()).filter(Boolean)),
  );
  const currentIds = new Set(currentTeams.map((team) => team.id));
  const nextIds = new Set(normalizedNextIds);

  return {
    toLink: normalizedNextIds.filter((teamId) => !currentIds.has(teamId)),
    toUnlink: currentTeams.map((team) => team.id).filter((teamId) => !nextIds.has(teamId)),
  };
}
