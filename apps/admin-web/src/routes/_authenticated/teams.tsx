import { useMutation } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate, useRouter } from "@tanstack/react-router";
import { type Dispatch, type FormEvent, type SetStateAction, useMemo, useState } from "react";

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
  validateSearch: (search: Record<string, unknown>) => ({
    teamId: typeof search.teamId === "string" ? search.teamId : undefined,
  }),
  loader: async () => {
    const gateways = await createGatewayBundle();
    const [teams, projects] = await Promise.all([
      gateways.teamGateway.listTeams(),
      gateways.projectGateway.listProjects(),
    ]);
    const members =
      teams.length === 0
        ? []
        : (await Promise.all(teams.map((team) => gateways.teamGateway.listMembersByTeam(team.id)))).flat();
    const linkedProjectsByTeam = (
      await Promise.all(
        projects.map(async (project) => ({
          project,
          teams: await gateways.teamGateway.listTeamsByProject(project.id),
        })),
      )
    ).reduce<Record<string, { id: string; name: string }[]>>((accumulator, entry) => {
      entry.teams.forEach((team) => {
        const currentProjects = accumulator[team.id] ?? [];
        currentProjects.push({ id: entry.project.id, name: entry.project.name });
        accumulator[team.id] = currentProjects;
      });
      return accumulator;
    }, {});

    return { teams, members, linkedProjectsByTeam };
  },
  component: TeamsPage,
});

function TeamsPage() {
  const { teams, members, linkedProjectsByTeam } = Route.useLoaderData();
  const { teamId: teamIdFromSearch } = Route.useSearch();
  const navigate = useNavigate();
  const router = useRouter();
  const [teamName, setTeamName] = useState("");
  const [editingTeamId, setEditingTeamId] = useState<string | null>(null);
  const [editingTeamName, setEditingTeamName] = useState("");
  const [editingMemberId, setEditingMemberId] = useState<string | null>(null);
  const [editingMemberTeamId, setEditingMemberTeamId] = useState("");
  const [memberSkillInput, setMemberSkillInput] = useState("");
  const [memberSkillTags, setMemberSkillTags] = useState<string[]>([]);
  const [memberEditSkillInput, setMemberEditSkillInput] = useState("");
  const [memberEditSkillTags, setMemberEditSkillTags] = useState<string[]>([]);
  const [memberForm, setMemberForm] = useState({
    teamId: teams[0]?.id ?? "",
    name: "",
    email: "",
    jiraAccountId: "",
    role: "backend" as MemberRole,
    levelLabel: "L3_middle" as LevelLabel,
    weeklyCapacityHours: 40,
  });
  const [memberEditForm, setMemberEditForm] = useState({
    name: "",
    email: "",
    jiraAccountId: "",
    role: "backend" as MemberRole,
    levelLabel: "L3_middle" as LevelLabel,
    weeklyCapacityHours: 40,
  });
  const activeTeamId = teams.some((team) => team.id === teamIdFromSearch)
    ? teamIdFromSearch
    : teams.some((team) => team.id === memberForm.teamId)
      ? memberForm.teamId
      : (teams[0]?.id ?? "");

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

  const renameTeam = useMutation({
    mutationFn: async () => {
      if (!editingTeamId) {
        return null;
      }

      const gateways = await createGatewayBundle();
      return gateways.teamGateway.updateTeam(editingTeamId, editingTeamName);
    },
    onSuccess: async () => {
      setEditingTeamId(null);
      setEditingTeamName("");
      await router.invalidate();
    },
  });

  const addMember = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      return gateways.teamGateway.addMember({
        teamId: activeTeamId,
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

  const updateMember = useMutation({
    mutationFn: async () => {
      if (!editingMemberId) {
        return null;
      }

      const gateways = await createGatewayBundle();
      return gateways.teamGateway.updateMember(editingMemberId, {
        teamId: editingMemberTeamId || undefined,
        name: memberEditForm.name,
        email: memberEditForm.email || null,
        jiraAccountId: memberEditForm.jiraAccountId || null,
        role: memberEditForm.role,
        levelLabel: memberEditForm.levelLabel,
        skillTags: memberEditSkillTags,
        weeklyCapacityHours: memberEditForm.weeklyCapacityHours,
      });
    },
    onSuccess: async () => {
      setEditingMemberId(null);
      setEditingMemberTeamId("");
      setMemberEditSkillInput("");
      setMemberEditSkillTags([]);
      await router.invalidate();
    },
  });

  const selectedTeam = teams.find((team) => team.id === activeTeamId) ?? null;
  const visibleMembers = useMemo(
    () => (selectedTeam ? members.filter((member) => member.teamId === selectedTeam.id) : members),
    [members, selectedTeam],
  );
  const capacitySummary = useMemo(
    () => members.reduce((sum, member) => sum + member.weeklyCapacityHours, 0),
    [members],
  );
  const memberCountsByTeam = useMemo(
    () =>
      members.reduce<Record<string, number>>((accumulator, member) => {
        accumulator[member.teamId] = (accumulator[member.teamId] ?? 0) + 1;
        return accumulator;
      }, {}),
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

  const startEditMember = (member: (typeof members)[number]) => {
    setEditingMemberId(member.id);
    setEditingMemberTeamId(member.teamId);
    setMemberEditForm({
      name: member.name,
      email: member.email ?? "",
      jiraAccountId: member.jiraAccountId ?? "",
      role: member.role,
      levelLabel: member.levelLabel,
      weeklyCapacityHours: member.weeklyCapacityHours,
    });
    setMemberEditSkillTags(member.skillTags);
    setMemberEditSkillInput("");
  };

  const selectTeam = (teamId: string) => {
    setMemberForm((current) => ({ ...current, teamId }));
    void navigate({
      replace: true,
      search: { teamId },
      to: "/teams",
    });
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
                  const active = team.id === activeTeamId;
                  const linkedProjects = linkedProjectsByTeam[team.id] ?? [];
                  const memberCount = memberCountsByTeam[team.id] ?? 0;

                  return (
                    <div
                      key={team.id}
                      className={`rounded-2xl border px-4 py-3 transition-colors ${
                        active
                          ? "border-accent/60 bg-accent/10 shadow-[0_14px_32px_rgba(32,79,59,0.14)]"
                          : "border-border bg-card"
                      }`}
                      onClick={() => selectTeam(team.id)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          selectTeam(team.id);
                        }
                      }}
                      role="button"
                      tabIndex={0}
                    >
                      <div className="flex flex-wrap items-center justify-between gap-3">
                        <div className="min-w-0">
                          {editingTeamId === team.id ? (
                            <input
                              autoFocus
                              className="w-full rounded-xl border border-border bg-background px-3 py-2 text-card-foreground"
                              onChange={(event) => setEditingTeamName(event.target.value)}
                              onClick={(event) => event.stopPropagation()}
                              value={editingTeamName}
                            />
                          ) : (
                            <>
                              <p className="font-medium">{team.name}</p>
                              <p className="text-sm text-muted-foreground">
                                {memberCount} members
                              </p>
                              <div className="mt-3 flex flex-wrap gap-2">
                                {linkedProjects.length === 0 ? (
                                  <span className="text-xs uppercase tracking-[0.16em] text-muted-foreground">
                                    Not linked to any project yet
                                  </span>
                                ) : (
                                  linkedProjects.map((project) => (
                                    <Link
                                      key={project.id}
                                      onClick={(event) => event.stopPropagation()}
                                      params={{ projectId: project.id }}
                                      to="/projects/$projectId"
                                    >
                                      <Badge tone={active ? "success" : "neutral"}>
                                        {project.name}
                                      </Badge>
                                    </Link>
                                  ))
                                )}
                              </div>
                            </>
                          )}
                        </div>
                        <div className="flex flex-wrap items-center gap-2">
                          {active ? <Badge tone="success">selected</Badge> : null}
                          {editingTeamId === team.id ? (
                            <>
                              <Button
                                className="active:scale-95"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  renameTeam.mutate();
                                }}
                                type="button"
                                variant="secondary"
                              >
                                Save
                              </Button>
                              <Button
                                className="active:scale-95"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  setEditingTeamId(null);
                                  setEditingTeamName("");
                                }}
                                type="button"
                                variant="ghost"
                              >
                                Cancel
                              </Button>
                            </>
                          ) : (
                            <Button
                              className={active ? "active:scale-95" : "active:scale-95"}
                              onClick={(event) => {
                                event.stopPropagation();
                                setEditingTeamId(team.id);
                                setEditingTeamName(team.name);
                              }}
                              type="button"
                              variant="secondary"
                            >
                              Rename
                            </Button>
                          )}
                          <Button
                            className="bg-danger text-white hover:bg-danger/90 active:scale-95"
                            onClick={(event) => {
                              event.stopPropagation();
                              deleteTeam.mutate(team.id);
                            }}
                            type="button"
                            variant="secondary"
                          >
                            Delete
                          </Button>
                        </div>
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
              value={activeTeamId}
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
            <div className="hidden gap-3 border-b border-border bg-card px-4 py-3 text-sm font-medium text-muted-foreground lg:grid lg:grid-cols-[minmax(0,1.2fr)_minmax(0,0.9fr)_minmax(0,0.9fr)_minmax(0,1fr)_auto_auto]">
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
                  className="border-b border-border bg-background/80 px-4 py-4 last:border-b-0"
                >
                  <div className="grid gap-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(0,0.9fr)_minmax(0,0.9fr)_minmax(0,1fr)_auto_auto] lg:items-start">
                    <div className="min-w-0">
                      <p className="font-medium">{member.name}</p>
                      <p className="text-sm text-muted-foreground">{member.email ?? "No email"}</p>
                    </div>
                    <div className="min-w-0">
                      <p className="mb-2 text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground lg:hidden">
                        Team
                      </p>
                      <Badge>{teams.find((team) => team.id === member.teamId)?.name ?? member.teamId}</Badge>
                    </div>
                    <div className="min-w-0">
                      <p className="mb-2 text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground lg:hidden">
                        Role
                      </p>
                      <span className="text-sm capitalize">{member.role.replaceAll("_", " ")}</span>
                    </div>
                    <div className="min-w-0">
                      <p className="mb-2 text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground lg:hidden">
                        Skills
                      </p>
                      <div className="flex flex-wrap gap-2">
                        {member.skillTags.length === 0 ? (
                          <span className="text-sm text-muted-foreground">No skills</span>
                        ) : (
                          member.skillTags.map((skill) => (
                            <Badge key={skill}>{skill}</Badge>
                          ))
                        )}
                      </div>
                    </div>
                    <div className="min-w-0">
                      <p className="mb-2 text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground lg:hidden">
                        Capacity
                      </p>
                      <Badge>{member.weeklyCapacityHours}h</Badge>
                    </div>
                    <div className="flex flex-wrap items-start justify-start gap-2 lg:justify-end">
                      <Button
                        className="active:scale-95"
                        onClick={() => startEditMember(member)}
                        type="button"
                        variant="secondary"
                      >
                        Edit
                      </Button>
                      <Button
                        className="bg-danger text-white hover:bg-danger/90 active:scale-95"
                        onClick={() => removeMember.mutate(member.id)}
                        type="button"
                        variant="secondary"
                      >
                        Remove
                      </Button>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </section>
        {editingMemberId ? (
          <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h2 className="text-xl font-semibold">Edit Member</h2>
                <p className="mt-2 text-sm text-muted-foreground">
                  Update member profile, team assignment, skills, and weekly capacity.
                </p>
              </div>
              <Badge tone="warning">editing</Badge>
            </div>
            <form
              className="mt-4 grid gap-3 md:grid-cols-2"
              onSubmit={(event) => {
                event.preventDefault();
                updateMember.mutate();
              }}
            >
              <input
                className="rounded-2xl border border-border bg-card px-4 py-3"
                onChange={(event) =>
                  setMemberEditForm((current) => ({ ...current, name: event.target.value }))
                }
                placeholder="Member name"
                required
                value={memberEditForm.name}
              />
              <input
                className="rounded-2xl border border-border bg-card px-4 py-3"
                onChange={(event) =>
                  setMemberEditForm((current) => ({ ...current, email: event.target.value }))
                }
                placeholder="Email"
                value={memberEditForm.email}
              />
              <input
                className="rounded-2xl border border-border bg-card px-4 py-3"
                onChange={(event) =>
                  setMemberEditForm((current) => ({
                    ...current,
                    jiraAccountId: event.target.value,
                  }))
                }
                placeholder="Jira account id"
                value={memberEditForm.jiraAccountId}
              />
              <select
                className="rounded-2xl border border-border bg-card px-4 py-3"
                onChange={(event) => setEditingMemberTeamId(event.target.value)}
                value={editingMemberTeamId}
              >
                <option value="">Select team</option>
                {teams.map((team) => (
                  <option key={team.id} value={team.id}>
                    {team.name}
                  </option>
                ))}
              </select>
              <select
                className="rounded-2xl border border-border bg-card px-4 py-3"
                onChange={(event) =>
                  setMemberEditForm((current) => ({
                    ...current,
                    role: event.target.value as MemberRole,
                  }))
                }
                value={memberEditForm.role}
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
                  setMemberEditForm((current) => ({
                    ...current,
                    levelLabel: event.target.value as LevelLabel,
                  }))
                }
                value={memberEditForm.levelLabel}
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
                  {memberEditSkillTags.length === 0 ? (
                    <span className="text-sm text-muted-foreground">No skills added</span>
                  ) : (
                    memberEditSkillTags.map((skill) => (
                      <button
                        key={skill}
                        className="rounded-full border border-border px-3 py-1 text-sm"
                        onClick={() =>
                          setMemberEditSkillTags((current) =>
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
                    onChange={(event) => setMemberEditSkillInput(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === ",") {
                        event.preventDefault();
                        appendSkillTag(
                          memberEditSkillInput,
                          setMemberEditSkillInput,
                          setMemberEditSkillTags,
                        );
                      }
                    }}
                    placeholder="Type a skill and press Enter"
                    value={memberEditSkillInput}
                  />
                  <Button
                    onClick={() =>
                      appendSkillTag(
                        memberEditSkillInput,
                        setMemberEditSkillInput,
                        setMemberEditSkillTags,
                      )
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
                  setMemberEditForm((current) => ({
                    ...current,
                    weeklyCapacityHours: Number(event.target.value),
                  }))
                }
                type="number"
                value={memberEditForm.weeklyCapacityHours}
              />
              <div className="flex items-center justify-end gap-2 md:col-span-2">
                <Button
                  onClick={() => {
                    setEditingMemberId(null);
                    setEditingMemberTeamId("");
                    setMemberEditSkillInput("");
                    setMemberEditSkillTags([]);
                  }}
                  type="button"
                  variant="secondary"
                >
                  Cancel
                </Button>
                <Button type="submit">Save member</Button>
              </div>
            </form>
          </section>
        ) : null}
      </div>
    </PageFrame>
  );
}
