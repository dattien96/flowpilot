import { useMutation } from "@tanstack/react-query";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { type Dispatch, type FormEvent, type SetStateAction, useMemo, useState } from "react";

import { createGatewayBundle } from "@/data/repository/browser-factory";
import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/components/ui/button";

export const Route = createFileRoute("/_authenticated/projects/$projectId/members")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const teams = await gateways.teamGateway.listTeamsByProject(params.projectId);
    const members =
      teams.length === 0
        ? []
        : (await Promise.all(teams.map((team) => gateways.teamGateway.listMembersByTeam(team.id)))).flat();
    const allTeams = await gateways.teamGateway.listTeams();
    return { teams, members, allTeams, projectId: params.projectId };
  },
  component: MembersPage,
});

function MembersPage() {
  const { teams, members, allTeams, projectId } = Route.useLoaderData();
  const router = useRouter();
  const [teamName, setTeamName] = useState("");
  const [editingTeamId, setEditingTeamId] = useState<string | null>(null);
  const [editingTeamName, setEditingTeamName] = useState("");
  const [editingMemberId, setEditingMemberId] = useState<string | null>(null);
  const [editingMemberTeamId, setEditingMemberTeamId] = useState("");
  const [memberForm, setMemberForm] = useState({
    teamId: teams[0]?.id ?? "",
    name: "",
    email: "",
    jiraAccountId: "",
    role: "backend" as "android" | "ios" | "backend" | "frontend" | "qa" | "devops" | "ai_workflow",
    levelLabel: "L3_middle" as "L1_intern" | "L2_junior" | "L3_middle" | "L4_senior" | "L5_lead",
    weeklyCapacityHours: 40,
  });
  const [memberSkillInput, setMemberSkillInput] = useState("");
  const [memberSkillTags, setMemberSkillTags] = useState<string[]>([]);
  const [memberEditForm, setMemberEditForm] = useState({
    name: "",
    email: "",
    role: "backend" as "android" | "ios" | "backend" | "frontend" | "qa" | "devops" | "ai_workflow",
    levelLabel: "L3_middle" as "L1_intern" | "L2_junior" | "L3_middle" | "L4_senior" | "L5_lead",
    weeklyCapacityHours: 40,
  });
  const [memberEditSkillInput, setMemberEditSkillInput] = useState("");
  const [memberEditSkillTags, setMemberEditSkillTags] = useState<string[]>([]);

  const createTeam = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      return gateways.teamGateway.createTeam(teamName);
    },
    onSuccess: async () => {
      setTeamName("");
      await router.invalidate();
    },
  });

  const renameTeam = useMutation({
    mutationFn: async () => {
      if (!editingTeamId) return null;
      const gateways = await createGatewayBundle();
      return gateways.teamGateway.updateTeam(editingTeamId, editingTeamName);
    },
    onSuccess: async () => {
      setEditingTeamId(null);
      setEditingTeamName("");
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
      setMemberForm((current) => ({ ...current, name: "", email: "", jiraAccountId: "" }));
      setMemberSkillTags([]);
      setMemberSkillInput("");
      await router.invalidate();
    },
  });

  const updateMember = useMutation({
    mutationFn: async () => {
      if (!editingMemberId) return null;
      const gateways = await createGatewayBundle();
      return gateways.teamGateway.updateMember(editingMemberId, {
        teamId: editingMemberTeamId || undefined,
        name: memberEditForm.name,
        email: memberEditForm.email || null,
        role: memberEditForm.role,
        levelLabel: memberEditForm.levelLabel,
        skillTags: memberEditSkillTags,
        weeklyCapacityHours: memberEditForm.weeklyCapacityHours,
      });
    },
    onSuccess: async () => {
      setEditingMemberId(null);
      setEditingMemberTeamId("");
      setMemberEditSkillTags([]);
      setMemberEditSkillInput("");
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

  const workloadSummary = useMemo(() => {
    const totalCapacity = members.reduce((sum, member) => sum + member.weeklyCapacityHours, 0);
    const assignedHours = members.reduce((sum, member) => sum + Math.max(0, member.weeklyCapacityHours - 8), 0);
    return {
      totalCapacity,
      assignedHours,
      utilization: totalCapacity === 0 ? 0 : Math.min(100, Math.round((assignedHours / totalCapacity) * 100)),
    };
  }, [members]);

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
      role: member.role,
      levelLabel: member.levelLabel,
      weeklyCapacityHours: member.weeklyCapacityHours,
    });
    setMemberEditSkillTags(member.skillTags);
    setMemberEditSkillInput("");
  };

  const appendSkillTag = (
    value: string,
    setInput: Dispatch<SetStateAction<string>>,
    setTags: Dispatch<SetStateAction<string[]>>,
  ) => {
    const tag = value.trim();
    if (!tag) return;
    setTags((current) => (current.includes(tag) ? current : [...current, tag]));
    setInput("");
  };

  return (
    <PageFrame description="Team member management for the selected project." title="Members">
      <div className="space-y-6">
        <ProjectSectionNav projectId={projectId} />

        <section className="grid gap-4 md:grid-cols-3">
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <p className="text-sm text-muted-foreground">Members</p>
            <p className="mt-2 text-3xl font-semibold">{members.length}</p>
          </div>
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <p className="text-sm text-muted-foreground">Capacity</p>
            <p className="mt-2 text-3xl font-semibold">{workloadSummary.totalCapacity}h</p>
          </div>
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <p className="text-sm text-muted-foreground">Assigned</p>
            <p className="mt-2 text-3xl font-semibold">{workloadSummary.assignedHours}h</p>
            <p className="mt-2 text-sm text-muted-foreground">Utilization {workloadSummary.utilization}%</p>
          </div>
        </section>

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <div className="flex items-center justify-between gap-3">
            <h2 className="text-xl font-semibold">Teams</h2>
            <Button disabled type="button" variant="secondary">
              Import from Jira
            </Button>
          </div>
          <form className="mt-4 flex gap-3" onSubmit={handleCreateTeam}>
            <input
              className="flex-1 rounded-2xl border border-border bg-card px-4 py-3"
              placeholder="New team name"
              value={teamName}
              onChange={(event) => setTeamName(event.target.value)}
              required
            />
            <Button type="submit">Create team</Button>
          </form>

          <div className="mt-4 space-y-3">
            {teams.length === 0 ? (
              <p className="text-sm text-muted-foreground">No teams are linked to this project yet.</p>
            ) : (
              teams.map((team) => (
                <div
                  key={team.id}
                  className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border bg-card px-4 py-3"
                >
                  {editingTeamId === team.id ? (
                    <input
                      className="flex-1 rounded-xl border border-border bg-background px-3 py-2"
                      value={editingTeamName}
                      onChange={(event) => setEditingTeamName(event.target.value)}
                    />
                  ) : (
                    <div>
                      <p className="font-medium">{team.name}</p>
                      <p className="text-sm text-muted-foreground">
                        {teams.some((linkedTeam) => linkedTeam.id === team.id) ? "linked to project" : "available"}
                      </p>
                    </div>
                  )}
                  <div className="flex gap-2">
                    {editingTeamId === team.id ? (
                      <Button onClick={() => renameTeam.mutate()} type="button" variant="secondary">
                        Save
                      </Button>
                    ) : (
                      <Button
                        onClick={() => {
                          setEditingTeamId(team.id);
                          setEditingTeamName(team.name);
                        }}
                        type="button"
                        variant="secondary"
                      >
                        Rename
                      </Button>
                    )}
                    <Button onClick={() => deleteTeam.mutate(team.id)} type="button" variant="destructive">
                      Delete
                    </Button>
                  </div>
                </div>
              ))
            )}
          </div>
        </section>

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <h2 className="text-xl font-semibold">Add Member</h2>
          <form className="mt-4 grid gap-3 md:grid-cols-2" onSubmit={handleAddMember}>
            <div className="flex items-center justify-between gap-3 rounded-2xl border border-border bg-card px-4 py-3 md:col-span-2">
              <div>
                <p className="font-medium">Import from Jira</p>
                <p className="text-sm text-muted-foreground">Disabled until the Jira MCP is connected in Phase 7.</p>
              </div>
              <Button disabled type="button" variant="secondary">
                Import from Jira
              </Button>
            </div>
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              placeholder="Name"
              value={memberForm.name}
              onChange={(event) => setMemberForm((current) => ({ ...current, name: event.target.value }))}
              required
            />
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              placeholder="Email"
              value={memberForm.email}
              onChange={(event) => setMemberForm((current) => ({ ...current, email: event.target.value }))}
            />
            <select
              className="rounded-2xl border border-border bg-card px-4 py-3"
              value={memberForm.teamId}
              onChange={(event) => setMemberForm((current) => ({ ...current, teamId: event.target.value }))}
            >
              <option value="">Select team</option>
              {allTeams.map((team) => (
                <option key={team.id} value={team.id}>
                  {team.name}
                </option>
              ))}
            </select>
            <select
              className="rounded-2xl border border-border bg-card px-4 py-3"
              value={memberForm.role}
              onChange={(event) =>
                setMemberForm((current) => ({ ...current, role: event.target.value as typeof current.role }))
              }
            >
              <option value="android">android</option>
              <option value="ios">ios</option>
              <option value="backend">backend</option>
              <option value="frontend">frontend</option>
              <option value="qa">qa</option>
              <option value="devops">devops</option>
              <option value="ai_workflow">ai_workflow</option>
            </select>
            <select
              className="rounded-2xl border border-border bg-card px-4 py-3"
              value={memberForm.levelLabel}
              onChange={(event) =>
                setMemberForm((current) => ({ ...current, levelLabel: event.target.value as typeof current.levelLabel }))
              }
            >
              <option value="L1_intern">L1_intern</option>
              <option value="L2_junior">L2_junior</option>
              <option value="L3_middle">L3_middle</option>
              <option value="L4_senior">L4_senior</option>
                <option value="L5_lead">L5_lead</option>
              </select>
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              placeholder="Jira account ID"
              value={memberForm.jiraAccountId}
              onChange={(event) => setMemberForm((current) => ({ ...current, jiraAccountId: event.target.value }))}
            />
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
                      onClick={() => setMemberSkillTags((current) => current.filter((item) => item !== skill))}
                      type="button"
                    >
                      {skill} ×
                    </button>
                  ))
                )}
              </div>
              <div className="mt-3 flex gap-2">
                <input
                  className="flex-1 rounded-2xl border border-border bg-background px-4 py-3"
                  placeholder="Type a skill and press Enter"
                  value={memberSkillInput}
                  onChange={(event) => setMemberSkillInput(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === ",") {
                      event.preventDefault();
                      appendSkillTag(memberSkillInput, setMemberSkillInput, setMemberSkillTags);
                    }
                  }}
                />
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => appendSkillTag(memberSkillInput, setMemberSkillInput, setMemberSkillTags)}
                >
                  Add
                </Button>
              </div>
            </div>
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              type="number"
              min={1}
              value={memberForm.weeklyCapacityHours}
              onChange={(event) =>
                setMemberForm((current) => ({ ...current, weeklyCapacityHours: Number(event.target.value) }))
              }
            />
            <div className="flex items-center justify-end md:col-span-2">
              <Button type="submit">Add member</Button>
            </div>
          </form>
        </section>

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <div className="flex items-center justify-between">
            <h2 className="text-xl font-semibold">Members</h2>
            <Badge>{members.length} members</Badge>
          </div>
          <div className="mt-4 overflow-hidden rounded-[1.25rem] border border-border">
            <div className="grid grid-cols-[1.2fr_0.8fr_0.8fr_1fr_0.7fr_0.7fr] gap-3 border-b border-border bg-card px-4 py-3 text-sm font-medium text-muted-foreground">
              <span>Name</span>
              <span>Role</span>
              <span>Level</span>
              <span>Skills</span>
              <span>Capacity</span>
              <span />
            </div>
            {members.length === 0 ? (
              <p className="px-4 py-4 text-sm text-muted-foreground">No team members are linked yet.</p>
            ) : (
              members.map((member) => (
                <div
                  key={member.id}
                  className="grid grid-cols-[1.2fr_0.8fr_0.8fr_1fr_0.7fr_0.7fr] gap-3 border-b border-border bg-background/80 px-4 py-4 last:border-b-0"
                >
                  <div>
                    <p className="font-medium">{member.name}</p>
                    <p className="text-sm text-muted-foreground">{member.email ?? "No email"}</p>
                  </div>
                  <span className="text-sm capitalize">{member.role.replaceAll("_", " ")}</span>
                  <Badge className="w-fit">{member.levelLabel}</Badge>
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
                  <div className="flex items-center justify-end gap-2">
                    <Button onClick={() => startEditMember(member)} type="button" variant="secondary">
                      Edit
                    </Button>
                    <Button variant="destructive" onClick={() => removeMember.mutate(member.id)} type="button">
                      Remove
                    </Button>
                  </div>
                </div>
              ))
            )}
          </div>
        </section>

        {editingMemberId ? (
          <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <h2 className="text-xl font-semibold">Edit Member</h2>
            <form
              className="mt-4 grid gap-3 md:grid-cols-2"
              onSubmit={(event) => {
                event.preventDefault();
                updateMember.mutate();
              }}
            >
              <input
                className="rounded-2xl border border-border bg-card px-4 py-3"
                placeholder="Name"
                value={memberEditForm.name}
                onChange={(event) => setMemberEditForm((current) => ({ ...current, name: event.target.value }))}
                required
              />
              <input
                className="rounded-2xl border border-border bg-card px-4 py-3"
                placeholder="Email"
                value={memberEditForm.email}
                onChange={(event) => setMemberEditForm((current) => ({ ...current, email: event.target.value }))}
              />
              <select
                className="rounded-2xl border border-border bg-card px-4 py-3"
                value={editingMemberTeamId}
                onChange={(event) => setEditingMemberTeamId(event.target.value)}
              >
                <option value="">Select team</option>
                {allTeams.map((team) => (
                  <option key={team.id} value={team.id}>
                    {team.name}
                  </option>
                ))}
              </select>
              <select
                className="rounded-2xl border border-border bg-card px-4 py-3"
                value={memberEditForm.role}
                onChange={(event) =>
                  setMemberEditForm((current) => ({ ...current, role: event.target.value as typeof current.role }))
                }
              >
                <option value="android">android</option>
                <option value="ios">ios</option>
                <option value="backend">backend</option>
                <option value="frontend">frontend</option>
                <option value="qa">qa</option>
                <option value="devops">devops</option>
                <option value="ai_workflow">ai_workflow</option>
              </select>
              <select
                className="rounded-2xl border border-border bg-card px-4 py-3"
                value={memberEditForm.levelLabel}
                onChange={(event) =>
                  setMemberEditForm((current) => ({
                    ...current,
                    levelLabel: event.target.value as typeof current.levelLabel,
                  }))
                }
              >
                <option value="L1_intern">L1_intern</option>
                <option value="L2_junior">L2_junior</option>
                <option value="L3_middle">L3_middle</option>
                <option value="L4_senior">L4_senior</option>
                <option value="L5_lead">L5_lead</option>
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
                        onClick={() => setMemberEditSkillTags((current) => current.filter((item) => item !== skill))}
                        type="button"
                      >
                        {skill} ×
                      </button>
                    ))
                  )}
                </div>
                <div className="mt-3 flex gap-2">
                  <input
                    className="flex-1 rounded-2xl border border-border bg-background px-4 py-3"
                    placeholder="Type a skill and press Enter"
                    value={memberEditSkillInput}
                    onChange={(event) => setMemberEditSkillInput(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === ",") {
                        event.preventDefault();
                        appendSkillTag(memberEditSkillInput, setMemberEditSkillInput, setMemberEditSkillTags);
                      }
                    }}
                  />
                  <Button
                    type="button"
                    variant="secondary"
                    onClick={() => appendSkillTag(memberEditSkillInput, setMemberEditSkillInput, setMemberEditSkillTags)}
                  >
                    Add
                  </Button>
                </div>
              </div>
              <input
                className="rounded-2xl border border-border bg-card px-4 py-3"
                type="number"
                min={1}
                value={memberEditForm.weeklyCapacityHours}
                onChange={(event) =>
                  setMemberEditForm((current) => ({ ...current, weeklyCapacityHours: Number(event.target.value) }))
                }
              />
              <div className="flex items-center justify-end gap-2 md:col-span-2">
                <Button type="button" variant="secondary" onClick={() => setEditingMemberId(null)}>
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
