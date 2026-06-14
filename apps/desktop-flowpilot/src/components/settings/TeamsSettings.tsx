import { useEffect, useMemo, useState } from "react";
import type { LevelLabel, MemberRole, Team, TeamMember } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { toErrorMessage } from "@/components/settings/settingsHelpers";

const roles: MemberRole[] = ["android", "ios", "backend", "frontend", "qa", "devops", "ai_workflow"];
const levels: LevelLabel[] = ["L1_intern", "L2_junior", "L3_middle", "L4_senior", "L5_lead"];

export function TeamsSettings(): React.ReactElement {
  const [teams, setTeams] = useState<Team[]>([]);
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [selectedTeamId, setSelectedTeamId] = useState("");
  const [teamName, setTeamName] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [member, setMember] = useState({
    name: "",
    email: "",
    jiraAccountId: "",
    role: "backend" as MemberRole,
    levelLabel: "L3_middle" as LevelLabel,
    weeklyCapacityHours: 40,
  });

  const refresh = async (teamId?: string) => {
    try {
      const admin = await getAdminUseCases();
      const nextTeams = await admin.teams.listTeams();
      const nextMembers = (await Promise.all(nextTeams.map((team) => admin.teams.listMembers(team.id)))).flat();
      setTeams(nextTeams);
      setMembers(nextMembers);
      setSelectedTeamId(teamId ?? nextTeams[0]?.id ?? "");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to load teams."));
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  const visibleMembers = useMemo(() => members.filter((item) => item.teamId === selectedTeamId), [members, selectedTeamId]);

  const createTeam = async () => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const team = await admin.teams.createTeam(teamName);
      setTeamName("");
      await refresh(team.id);
      setMessage("Team created.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to create team."));
    } finally {
      setBusy(false);
    }
  };

  const addMember = async () => {
    if (!selectedTeamId) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      await admin.teams.addMember({
        teamId: selectedTeamId,
        name: member.name,
        email: member.email || null,
        jiraAccountId: member.jiraAccountId || null,
        role: member.role,
        levelLabel: member.levelLabel,
        skillTags: [],
        weeklyCapacityHours: member.weeklyCapacityHours,
      });
      setMember({ name: "", email: "", jiraAccountId: "", role: "backend", levelLabel: "L3_middle", weeklyCapacityHours: 40 });
      await refresh(selectedTeamId);
      setMessage("Member added.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to add member."));
    } finally {
      setBusy(false);
    }
  };

  const removeMember = async (memberId: string) => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      await admin.teams.removeMember(memberId);
      await refresh(selectedTeamId);
      setMessage("Member removed.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to remove member."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head"><div><div className="settings-eyebrow">Teams</div><h2>Teams</h2><p>Create teams and manage team members.</p></div></div>
      {message ? <div className="settings-feedback">{message}</div> : null}
      <div className="settings-two-column">
        <div className="settings-subpanel">
          <h3>Teams</h3>
          <div className="settings-actions"><input className="settings-inline-input" placeholder="Team name" value={teamName} onChange={(event) => setTeamName(event.target.value)} /><button className="primary-btn" disabled={busy} onClick={() => void createTeam()} type="button">Create</button></div>
          <div className="settings-list">{teams.map((team) => <button className={`settings-list-item ${team.id === selectedTeamId ? "active" : ""}`} key={team.id} onClick={() => setSelectedTeamId(team.id)} type="button"><strong>{team.name}</strong><span>{members.filter((entry) => entry.teamId === team.id).length} members</span></button>)}</div>
        </div>
        <div className="settings-subpanel">
          <h3>Add Member</h3>
          <div className="settings-grid">
            <label className="settings-field"><span>Name</span><input value={member.name} onChange={(event) => setMember((current) => ({ ...current, name: event.target.value }))} /></label>
            <label className="settings-field"><span>Email</span><input value={member.email} onChange={(event) => setMember((current) => ({ ...current, email: event.target.value }))} /></label>
            <label className="settings-field"><span>Jira Account</span><input value={member.jiraAccountId} onChange={(event) => setMember((current) => ({ ...current, jiraAccountId: event.target.value }))} /></label>
            <label className="settings-field"><span>Role</span><select value={member.role} onChange={(event) => setMember((current) => ({ ...current, role: event.target.value as MemberRole }))}>{roles.map((role) => <option key={role} value={role}>{role}</option>)}</select></label>
            <label className="settings-field"><span>Level</span><select value={member.levelLabel} onChange={(event) => setMember((current) => ({ ...current, levelLabel: event.target.value as LevelLabel }))}>{levels.map((level) => <option key={level} value={level}>{level}</option>)}</select></label>
            <label className="settings-field"><span>Capacity</span><input min={1} type="number" value={member.weeklyCapacityHours} onChange={(event) => setMember((current) => ({ ...current, weeklyCapacityHours: Number(event.target.value) || 1 }))} /></label>
          </div>
          <div className="settings-actions"><button className="primary-btn" disabled={busy || !selectedTeamId} onClick={() => void addMember()} type="button">Add Member</button></div>
        </div>
      </div>
      <div className="settings-subpanel"><h3>Members</h3><div className="settings-list">{visibleMembers.map((item) => <div className="settings-list-item static" key={item.id}><div><strong>{item.name}</strong><span>{item.role} / {item.levelLabel} / {item.weeklyCapacityHours}h</span></div><button className="ghost-btn" disabled={busy} onClick={() => void removeMember(item.id)} type="button">Remove</button></div>)}</div></div>
    </section>
  );
}
