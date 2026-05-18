import { useMutation } from "@tanstack/react-query";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { type Dispatch, type FormEvent, type SetStateAction, useEffect, useMemo, useState } from "react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { LevelLabel, MemberRole } from "@/domain/model/entity/team";
import { Badge } from "@/presentation/components/ui/badge";

const roleOptions: MemberRole[] = [
  "android",
  "ios",
  "backend",
  "frontend",
  "qa",
  "devops",
  "ai_workflow",
];

const levelOptions: LevelLabel[] = [
  "L1_intern",
  "L2_junior",
  "L3_middle",
  "L4_senior",
  "L5_lead",
];

export const Route = createFileRoute("/_authenticated/teams")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const teams = await gateways.teamGateway.listTeams();
    const members =
      teams.length === 0
        ? []
        : (await Promise.all(teams.map((team) => gateways.teamGateway.listMembersByTeam(team.id)))).flat();

    return { teams, members };
  },
  component: TeamsPage,
});

function TeamsPage() {
  const { teams, members } = Route.useLoaderData();
  const router = useRouter();
  const [teamName, setTeamName] = useState("");
  const [memberSkillInput, setMemberSkillInput] = useState("");
  const [memberSkillTags, setMemberSkillTags] = useState<string[]>([]);
  const [memberForm, setMemberForm] = useState({
    teamId: teams[0]?.id ?? "",
    name: "",
    email: "",
    jiraAccountId: "",
    role: "backend" as MemberRole,
    levelLabel: "L3_middle" as LevelLabel,
    weeklyCapacityHours: 40,
  });

  useEffect(() => {
    setMemberForm((current) => {
      if (teams.some((team) => team.id === current.teamId)) {
        return current;
      }

      return {
        ...current,
        teamId: teams[0]?.id ?? "",
      };
    });
  }, [teams]);

  const createTeam = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      return gateways.teamGateway.createTeam(teamName);
    },
    onSuccess: async (team) => {
      setTeamName("");
      setMemberForm((current) => ({ ...current, teamId: team.id }));
      await router.invalidate();
    },
  });

  const deleteTeam = useMutation({
    mutationFn: async (teamId: string) => {
      const gateways = await createGatewayBundle();
      await gateways.teamGateway.deleteTeam(teamId);
    },
    onSuccess: async () => {
      await router.invalidate();
    },
  });

  const addMember = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      return gateways.teamGateway.addMember({
        teamId: memberForm.teamId,
        name: memberForm.name,
        email: memberForm.email || null,
        jiraAccountId: memberForm.jiraAccountId || null,
        role: memberForm.role,
        levelLabel: memberForm.levelLabel,
        skillTags: memberSkillTags,
        weeklyCapacityHours: memberForm.weeklyCapacityHours,
      });
    },
    onSuccess: async () => {
      setMemberForm((current) => ({
        ...current,
        name: "",
        email: "",
        jiraAccountId: "",
      }));
      setMemberSkillInput("");
      setMemberSkillTags([]);
      await router.invalidate();
    },
  });

  const removeMember = useMutation({
    mutationFn: async (memberId: string) => {
      const gateways = await createGatewayBundle();
      await gateways.teamGateway.removeMember(memberId);
    },
    onSuccess: async () => {
      await router.invalidate();
    },
  });

  const selectedTeam = teams.find((team) => team.id === memberForm.teamId) ?? null;
  const visibleMembers = useMemo(
    () => (selectedTeam ? members.filter((member) => member.teamId === selectedTeam.id) : members),
    [members, selectedTeam],
  );
  const capacitySummary = useMemo(
    () => members.reduce((sum, member) => sum + member.weeklyCapacityHours, 0),
    [members],
  );

  const handleCreateTeam = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    createTeam.mutate();
  };

  const handleAddMember = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    addMember.mutate();
  };

  const appendSkillTag = (
    value: string,
    setInput: Dispatch<SetStateAction<string>>,
    setTags: Dispatch<SetStateAction<string[]>>,
  ) => {
    const tag = value.trim();
    if (!tag) {
      return;
    }

    setTags((current) => (current.includes(tag) ? current : [...current, tag]));
    setInput("");
  };

  return (
    <PageFrame
      description="Create shared teams and attach members from a top-level workspace route."
      title="Teams"
    >
      <div className="space-y-6">
        <section className="grid gap-4 md:grid-cols-3">
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <p className="text-sm text-muted-foreground">Teams</p>
            <p className="mt-2 text-3xl font-semibold">{teams.length}</p>
          </div>
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <p className="text-sm text-muted-foreground">Members</p>
            <p className="mt-2 text-3xl font-semibold">{members.length}</p>
          </div>
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <p className="text-sm text-muted-foreground">Capacity</p>
            <p className="mt-2 text-3xl font-semibold">{capacitySummary}h</p>
          </div>
        </section>

        <section className="grid gap-4 xl:grid-cols-[0.9fr_1.1fr]">
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <h2 className="text-xl font-semibold">Create Team</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              Add a reusable team that can later be linked to one or more projects.
            </p>
            <form className="mt-4 flex gap-3" onSubmit={handleCreateTeam}>
              <input
                className="flex-1 rounded-2xl border border-border bg-card px-4 py-3"
                onChange={(event) => setTeamName(event.target.value)}
                placeholder="New team name"
                required
                value={teamName}
              />
              <Button disabled={createTeam.isPending} type="submit">
                Create team
              </Button>
            </form>
          </div>

          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h2 className="text-xl font-semibold">Team Directory</h2>
                <p className="mt-2 text-sm text-muted-foreground">
                  Select a team below to target new member creation.
                </p>
              </div>
              <Badge>{teams.length} total</Badge>
            </div>
            <div className="mt-4 space-y-3">
              {teams.length === 0 ? (
                <p className="text-sm text-muted-foreground">No teams created yet.</p>
              ) : (
                teams.map((team) => {
                  const active = team.id === memberForm.teamId;

                  return (
                    <div
                      key={team.id}
                      className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border bg-card px-4 py-3"
                    >
                      <div>
                        <p className="font-medium">{team.name}</p>
                        <p className="text-sm text-muted-foreground">
                          {members.filter((member) => member.teamId === team.id).length} members
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        {active ? <Badge>target</Badge> : null}
                        <Button
                          onClick={() =>
                            setMemberForm((current) => ({ ...current, teamId: team.id }))
                          }
                          type="button"
                          variant="secondary"
                        >
                          {active ? "Selected" : "Select"}
                        </Button>
                        <Button
                          onClick={() => deleteTeam.mutate(team.id)}
                          type="button"
                          variant="destructive"
                        >
                          Delete
                        </Button>
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        </section>

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <div className="flex items-center justify-between gap-3">
            <div>
              <h2 className="text-xl font-semibold">Add Team Member</h2>
              <p className="mt-2 text-sm text-muted-foreground">
                Attach a new member to any team from the workspace menu.
              </p>
            </div>
            {selectedTeam ? <Badge>{selectedTeam.name}</Badge> : null}
          </div>
          <form className="mt-4 grid gap-3 md:grid-cols-2" onSubmit={handleAddMember}>
            <select
              className="rounded-2xl border border-border bg-card px-4 py-3"
              onChange={(event) =>
                setMemberForm((current) => ({ ...current, teamId: event.target.value }))
              }
              required
              value={memberForm.teamId}
            >
              <option value="">Select team</option>
              {teams.map((team) => (
                <option key={team.id} value={team.id}>
                  {team.name}
                </option>
              ))}
            </select>
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              onChange={(event) =>
                setMemberForm((current) => ({ ...current, name: event.target.value }))
              }
              placeholder="Member name"
              required
              value={memberForm.name}
            />
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              onChange={(event) =>
                setMemberForm((current) => ({ ...current, email: event.target.value }))
              }
              placeholder="Email"
              value={memberForm.email}
            />
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              onChange={(event) =>
                setMemberForm((current) => ({ ...current, jiraAccountId: event.target.value }))
              }
              placeholder="Jira account id"
              value={memberForm.jiraAccountId}
            />
            <select
              className="rounded-2xl border border-border bg-card px-4 py-3"
              onChange={(event) =>
                setMemberForm((current) => ({
                  ...current,
                  role: event.target.value as MemberRole,
                }))
              }
              value={memberForm.role}
            >
              {roleOptions.map((role) => (
                <option key={role} value={role}>
                  {role}
                </option>
              ))}
            </select>
            <select
              className="rounded-2xl border border-border bg-card px-4 py-3"
              onChange={(event) =>
                setMemberForm((current) => ({
                  ...current,
                  levelLabel: event.target.value as LevelLabel,
                }))
              }
              value={memberForm.levelLabel}
            >
              {levelOptions.map((level) => (
                <option key={level} value={level}>
                  {level}
                </option>
              ))}
            </select>
            <div className="rounded-2xl border border-border bg-card px-4 py-3 md:col-span-2">
              <p className="text-sm font-medium">Skills</p>
              <div className="mt-3 flex flex-wrap gap-2">
                {memberSkillTags.length === 0 ? (
                  <span className="text-sm text-muted-foreground">No skills added</span>
                ) : (
                  memberSkillTags.map((skill) => (
                    <button
                      key={skill}
                      className="rounded-full border border-border px-3 py-1 text-sm"
                      onClick={() =>
                        setMemberSkillTags((current) =>
                          current.filter((existingSkill) => existingSkill !== skill),
                        )
                      }
                      type="button"
                    >
                      {skill} x
                    </button>
                  ))
                )}
              </div>
              <div className="mt-3 flex gap-2">
                <input
                  className="flex-1 rounded-2xl border border-border bg-background px-4 py-3"
                  onChange={(event) => setMemberSkillInput(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === ",") {
                      event.preventDefault();
                      appendSkillTag(memberSkillInput, setMemberSkillInput, setMemberSkillTags);
                    }
                  }}
                  placeholder="Type a skill and press Enter"
                  value={memberSkillInput}
                />
                <Button
                  onClick={() =>
                    appendSkillTag(memberSkillInput, setMemberSkillInput, setMemberSkillTags)
                  }
                  type="button"
                  variant="secondary"
                >
                  Add
                </Button>
              </div>
            </div>
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              min={1}
              onChange={(event) =>
                setMemberForm((current) => ({
                  ...current,
                  weeklyCapacityHours: Number(event.target.value),
                }))
              }
              type="number"
              value={memberForm.weeklyCapacityHours}
            />
            <div className="flex items-center justify-end md:col-span-2">
              <Button disabled={teams.length === 0 || addMember.isPending} type="submit">
                Add member
              </Button>
            </div>
          </form>
        </section>

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <div className="flex items-center justify-between gap-3">
            <div>
              <h2 className="text-xl font-semibold">Team Roster</h2>
              <p className="mt-2 text-sm text-muted-foreground">
                {selectedTeam
                  ? `Members currently attached to ${selectedTeam.name}.`
                  : "Create a team first to start building the roster."}
              </p>
            </div>
            <Badge>{visibleMembers.length} visible</Badge>
          </div>
          <div className="mt-4 overflow-hidden rounded-[1.25rem] border border-border">
            <div className="grid grid-cols-[1.2fr_0.9fr_0.9fr_1fr_0.8fr_0.7fr] gap-3 border-b border-border bg-card px-4 py-3 text-sm font-medium text-muted-foreground">
              <span>Name</span>
              <span>Team</span>
              <span>Role</span>
              <span>Skills</span>
              <span>Capacity</span>
              <span />
            </div>
            {visibleMembers.length === 0 ? (
              <p className="px-4 py-4 text-sm text-muted-foreground">No members available for this view yet.</p>
            ) : (
              visibleMembers.map((member) => (
                <div
                  key={member.id}
                  className="grid grid-cols-[1.2fr_0.9fr_0.9fr_1fr_0.8fr_0.7fr] gap-3 border-b border-border bg-background/80 px-4 py-4 last:border-b-0"
                >
                  <div>
                    <p className="font-medium">{member.name}</p>
                    <p className="text-sm text-muted-foreground">{member.email ?? "No email"}</p>
                  </div>
                  <Badge className="w-fit">
                    {teams.find((team) => team.id === member.teamId)?.name ?? member.teamId}
                  </Badge>
                  <span className="text-sm capitalize">{member.role.replaceAll("_", " ")}</span>
                  <div className="flex flex-wrap gap-2">
                    {member.skillTags.length === 0 ? (
                      <span className="text-sm text-muted-foreground">No skills</span>
                    ) : (
                      member.skillTags.map((skill) => (
                        <Badge key={skill} variant="secondary">
                          {skill}
                        </Badge>
                      ))
                    )}
                  </div>
                  <Badge>{member.weeklyCapacityHours}h</Badge>
                  <div className="flex items-center justify-end">
                    <Button
                      onClick={() => removeMember.mutate(member.id)}
                      type="button"
                      variant="destructive"
                    >
                      Remove
                    </Button>
                  </div>
                </div>
              ))
            )}
          </div>
        </section>
      </div>
    </PageFrame>
  );
}
