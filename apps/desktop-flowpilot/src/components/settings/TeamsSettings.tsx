import { useEffect, useMemo, useState } from "react";
import type { LevelLabel, MemberRole, Team, TeamMember } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { formatTimestamp, toErrorMessage } from "@/components/settings/settingsHelpers";

const ROLES: MemberRole[] = ["android", "ios", "backend", "frontend", "qa", "devops", "ai_workflow"];
const LEVELS: LevelLabel[] = ["L1_intern", "L2_junior", "L3_middle", "L4_senior", "L5_lead"];

type ViewMode = "list" | "create";

type MemberDraft = {
  name: string;
  email: string;
  jiraAccountId: string;
  role: MemberRole;
  levelLabel: LevelLabel;
  skillTagsText: string;
  weeklyCapacityHours: number;
};

const EMPTY_MEMBER_DRAFT: MemberDraft = {
  name: "",
  email: "",
  jiraAccountId: "",
  role: "backend",
  levelLabel: "L3_middle",
  skillTagsText: "",
  weeklyCapacityHours: 40,
};

function memberToMemberDraft(member: TeamMember): MemberDraft {
  return {
    name: member.name,
    email: member.email ?? "",
    jiraAccountId: member.jiraAccountId ?? "",
    role: member.role,
    levelLabel: member.levelLabel,
    skillTagsText: member.skillTags.join(", "),
    weeklyCapacityHours: member.weeklyCapacityHours,
  };
}

function parseMemberDraft(draft: MemberDraft): Omit<TeamMember, "id" | "teamId" | "createdAt" | "updatedAt"> {
  return {
    name: draft.name.trim(),
    email: draft.email.trim() || null,
    jiraAccountId: draft.jiraAccountId.trim() || null,
    role: draft.role,
    levelLabel: draft.levelLabel,
    skillTags: draft.skillTagsText
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean),
    weeklyCapacityHours: draft.weeklyCapacityHours,
  };
}

export function TeamsSettings(): React.ReactElement {
  const [viewMode, setViewMode] = useState<ViewMode>("list");
  const [teams, setTeams] = useState<Team[]>([]);
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [selectedTeamId, setSelectedTeamId] = useState("");
  const [teamNameDraft, setTeamNameDraft] = useState("");
  const [teamInitialName, setTeamInitialName] = useState("");
  const [createName, setCreateName] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; label: string } | null>(null);
  const [deleteConfirmationText, setDeleteConfirmationText] = useState("");
  const [memberModal, setMemberModal] = useState<"add" | TeamMember | null>(null);
  const [memberDraft, setMemberDraft] = useState<MemberDraft>(EMPTY_MEMBER_DRAFT);
  const [memberInitialSnapshot, setMemberInitialSnapshot] = useState("");

  const selectedTeam = useMemo(
    () => teams.find((team) => team.id === selectedTeamId) ?? null,
    [teams, selectedTeamId],
  );
  const teamNameDirty = teamNameDraft !== teamInitialName;
  const memberDirty =
    memberModal !== null &&
    memberModal !== "add" &&
    JSON.stringify(memberDraft) !== memberInitialSnapshot;

  const refresh = async (teamId?: string) => {
    try {
      const admin = await getAdminUseCases();
      const nextTeams = await admin.teams.listTeams();
      const resolvedId =
        teamId && nextTeams.some((t) => t.id === teamId)
          ? teamId
          : nextTeams[0]?.id ?? "";
      setTeams(nextTeams);
      setSelectedTeamId(resolvedId);
      const team = nextTeams.find((t) => t.id === resolvedId) ?? null;
      setTeamNameDraft(team?.name ?? "");
      setTeamInitialName(team?.name ?? "");
      const allMembers = (
        await Promise.all(nextTeams.map((t) => admin.teams.listMembers(t.id)))
      ).flat();
      setMembers(allMembers);
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to load teams."));
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  const selectTeam = (teamId: string) => {
    setMessage(null);
    const team = teams.find((t) => t.id === teamId) ?? null;
    setSelectedTeamId(teamId);
    setTeamNameDraft(team?.name ?? "");
    setTeamInitialName(team?.name ?? "");
  };

  const saveTeamName = async () => {
    if (!selectedTeamId || !teamNameDraft.trim()) return;
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      await admin.teams.updateTeam(selectedTeamId, teamNameDraft.trim());
      setTeamInitialName(teamNameDraft.trim());
      setTeams((current) =>
        current.map((t) =>
          t.id === selectedTeamId ? { ...t, name: teamNameDraft.trim() } : t,
        ),
      );
      setMessage("Team saved.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save team."));
    } finally {
      setBusy(false);
    }
  };

  const startCreate = () => {
    setCreateName("");
    setViewMode("create");
    setMessage(null);
  };

  const saveNewTeam = async () => {
    if (!createName.trim()) return;
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const team = await admin.teams.createTeam(createName.trim());
      setViewMode("list");
      await refresh(team.id);
      setMessage("Team created.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to create team."));
    } finally {
      setBusy(false);
    }
  };

  const deleteTeam = async () => {
    if (!deleteTarget) return;
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      await admin.teams.deleteTeam(deleteTarget.id);
      setDeleteTarget(null);
      setDeleteConfirmationText("");
      await refresh();
      setMessage("Team deleted.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to delete team."));
    } finally {
      setBusy(false);
    }
  };

  const openAddMember = () => {
    setMemberDraft(EMPTY_MEMBER_DRAFT);
    setMemberModal("add");
    setMessage(null);
  };

  const openMemberDetail = (member: TeamMember) => {
    const draft = memberToMemberDraft(member);
    setMemberDraft(draft);
    setMemberInitialSnapshot(JSON.stringify(draft));
    setMemberModal(member);
    setMessage(null);
  };

  const saveMember = async () => {
    if (!selectedTeamId) return;
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const payload = parseMemberDraft(memberDraft);
      if (memberModal === "add") {
        await admin.teams.addMember({ teamId: selectedTeamId, ...payload });
        setMessage("Member added.");
      } else if (memberModal !== null) {
        await admin.teams.updateMember(memberModal.id, payload);
        setMessage("Member saved.");
      }
      setMemberModal(null);
      const nextTeamMembers = await admin.teams.listMembers(selectedTeamId);
      setMembers((current) => [
        ...current.filter((m) => m.teamId !== selectedTeamId),
        ...nextTeamMembers,
      ]);
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save member."));
    } finally {
      setBusy(false);
    }
  };

  const removeMember = async (memberId: string) => {
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      await admin.teams.removeMember(memberId);
      setMemberModal(null);
      const nextTeamMembers = await admin.teams.listMembers(selectedTeamId);
      setMembers((current) => [
        ...current.filter((m) => m.teamId !== selectedTeamId),
        ...nextTeamMembers,
      ]);
      setMessage("Member removed.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to remove member."));
    } finally {
      setBusy(false);
    }
  };

  const renderMemberFields = () => (
    <div className="settings-grid">
      <label className="settings-field">
        <span>Name</span>
        <input
          onChange={(event) =>
            setMemberDraft((d) => ({ ...d, name: event.target.value }))
          }
          value={memberDraft.name}
        />
      </label>
      <label className="settings-field">
        <span>Email</span>
        <input
          onChange={(event) =>
            setMemberDraft((d) => ({ ...d, email: event.target.value }))
          }
          type="email"
          value={memberDraft.email}
        />
      </label>
      <label className="settings-field">
        <span>Jira Account ID</span>
        <input
          onChange={(event) =>
            setMemberDraft((d) => ({ ...d, jiraAccountId: event.target.value }))
          }
          value={memberDraft.jiraAccountId}
        />
      </label>
      <label className="settings-field">
        <span>Role</span>
        <select
          onChange={(event) =>
            setMemberDraft((d) => ({ ...d, role: event.target.value as MemberRole }))
          }
          value={memberDraft.role}
        >
          {ROLES.map((role) => (
            <option key={role} value={role}>
              {role}
            </option>
          ))}
        </select>
      </label>
      <label className="settings-field">
        <span>Level</span>
        <select
          onChange={(event) =>
            setMemberDraft((d) => ({ ...d, levelLabel: event.target.value as LevelLabel }))
          }
          value={memberDraft.levelLabel}
        >
          {LEVELS.map((level) => (
            <option key={level} value={level}>
              {level}
            </option>
          ))}
        </select>
      </label>
      <label className="settings-field">
        <span>Weekly capacity (hours)</span>
        <input
          min={1}
          onChange={(event) =>
            setMemberDraft((d) => ({
              ...d,
              weeklyCapacityHours: Number(event.target.value) || 1,
            }))
          }
          type="number"
          value={memberDraft.weeklyCapacityHours}
        />
      </label>
      <label className="settings-field settings-field-full">
        <span>Skills</span>
        <input
          onChange={(event) =>
            setMemberDraft((d) => ({ ...d, skillTagsText: event.target.value }))
          }
          placeholder="react, typescript, node"
          value={memberDraft.skillTagsText}
        />
      </label>
    </div>
  );

  const renderMemberModal = () => {
    if (!memberModal) return null;
    const isAdd = memberModal === "add";
    const title = isAdd ? "Add Member" : (memberModal as TeamMember).name;
    return (
      <div className="settings-modal-backdrop" role="presentation">
        <div className="settings-modal">
          <div className="project-create-head">
            <div>
              <div className="settings-eyebrow">
                {isAdd ? "Add" : "Member Detail"}
              </div>
              <h3>{title}</h3>
              {!isAdd && (
                <p className="project-muted-copy">
                  Edit member details. Save only becomes active after a change.
                </p>
              )}
            </div>
            <div className="settings-inline-actions">
              {!isAdd && (
                <button
                  className="secondary-btn"
                  disabled={busy}
                  onClick={() => void removeMember((memberModal as TeamMember).id)}
                  type="button"
                >
                  Remove
                </button>
              )}
              <button
                className="secondary-btn"
                onClick={() => setMemberModal(null)}
                type="button"
              >
                Cancel
              </button>
              <button
                className="primary-btn"
                disabled={busy || !memberDraft.name.trim() || (!isAdd && !memberDirty)}
                onClick={() => void saveMember()}
                type="button"
              >
                {isAdd ? "Add Member" : "Save Member"}
              </button>
            </div>
          </div>
          {renderMemberFields()}
        </div>
      </div>
    );
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head">
        <div>
          <div className="settings-eyebrow">Teams</div>
          <h2>Teams</h2>
          <p>Manage teams and team members.</p>
        </div>
      </div>

      {message ? (
        <div
          className={`settings-feedback ${message.toLowerCase().includes("unable") ? "error" : ""}`}
        >
          {message}
        </div>
      ) : null}

      {viewMode === "create" ? (
        <div className="settings-subpanel" style={{ marginTop: 18 }}>
          <div className="project-create-back-row">
            <button
              aria-label="Back to teams"
              className="secondary-btn project-icon-btn"
              onClick={() => setViewMode("list")}
              title="Back to teams"
              type="button"
            >
              &lt;
            </button>
          </div>
          <div className="project-create-head">
            <div>
              <div className="settings-eyebrow">Create Team</div>
              <h3>New Team</h3>
              <p className="project-muted-copy">
                Create the team first, then add members from the detail view.
              </p>
            </div>
            <button
              className="primary-btn"
              disabled={busy || !createName.trim()}
              onClick={() => void saveNewTeam()}
              type="button"
            >
              Save Team
            </button>
          </div>
          <div className="settings-grid">
            <label className="settings-field">
              <span>Team name</span>
              <input
                onChange={(event) => setCreateName(event.target.value)}
                placeholder="My Team"
                value={createName}
              />
            </label>
          </div>
        </div>
      ) : (
        <div className="project-layout">
          <aside className="settings-subpanel project-registry-panel">
            <div className="project-registry-head">
              <div>
                <div className="settings-eyebrow">Team Registry</div>
                <h3>Teams</h3>
                <p className="project-muted-copy">
                  Select a team to view and edit its detail.
                </p>
              </div>
              <button
                aria-label="Create team"
                className="primary-btn project-create-fab"
                onClick={startCreate}
                type="button"
              >
                +
              </button>
            </div>
            <div className="settings-list">
              {teams.length === 0 ? (
                <div className="settings-empty">
                  No teams found. Use the + button to create one.
                </div>
              ) : (
                teams.map((team) => (
                  <button
                    className={`settings-list-item ${team.id === selectedTeamId ? "active" : ""}`}
                    key={team.id}
                    onClick={() => selectTeam(team.id)}
                    type="button"
                  >
                    <div>
                      <strong>{team.name}</strong>
                      <span>
                        {members.filter((m) => m.teamId === team.id).length} members /{" "}
                        {formatTimestamp(team.updatedAt)}
                      </span>
                    </div>
                  </button>
                ))
              )}
            </div>
          </aside>

          <div className="project-detail-column">
            {selectedTeam ? (
              <>
                <div className="settings-subpanel project-detail-panel">
                  <div className="project-create-head">
                    <div>
                      <div className="settings-eyebrow">Team Detail</div>
                      <h3>{selectedTeam.name}</h3>
                      <p className="project-muted-copy">
                        Edit team name. Save only becomes active after a change.
                      </p>
                    </div>
                    <div className="settings-inline-actions">
                      <button
                        className="secondary-btn"
                        onClick={() =>
                          setDeleteTarget({ id: selectedTeamId, label: selectedTeam.name })
                        }
                        type="button"
                      >
                        Delete
                      </button>
                      <button
                        className="primary-btn"
                        disabled={busy || !teamNameDirty || !teamNameDraft.trim()}
                        onClick={() => void saveTeamName()}
                        type="button"
                      >
                        Save Team
                      </button>
                    </div>
                  </div>
                  <div className="settings-grid">
                    <label className="settings-field">
                      <span>Team name</span>
                      <input
                        onChange={(event) => setTeamNameDraft(event.target.value)}
                        value={teamNameDraft}
                      />
                    </label>
                  </div>
                </div>

                <div className="settings-subpanel project-detail-panel" style={{ marginTop: 16 }}>
                  <div className="project-panel-head">
                    <div>
                      <strong>Members</strong>
                      <p className="project-muted-copy">
                        Click a member to view or edit their details.
                      </p>
                    </div>
                    <button
                      className="primary-btn project-create-fab"
                      onClick={openAddMember}
                      type="button"
                    >
                      +
                    </button>
                  </div>
                  <div className="settings-list" style={{ marginTop: 12 }}>
                    {members.length === 0 ? (
                      <div className="settings-empty">
                        No members yet. Use the + button to add one.
                      </div>
                    ) : (
                      members.map((m) => (
                        <button
                          className="settings-list-item"
                          key={m.id}
                          onClick={() => openMemberDetail(m)}
                          type="button"
                        >
                          <div>
                            <strong>{m.name}</strong>
                            <span>
                              {m.role} / {m.levelLabel} / {m.weeklyCapacityHours}h
                              {m.skillTags.length > 0 ? ` / ${m.skillTags.join(", ")}` : ""}
                            </span>
                          </div>
                        </button>
                      ))
                    )}
                  </div>
                </div>
              </>
            ) : (
              <div className="settings-subpanel">
                <div className="settings-empty">
                  Select a team to inspect its detail page.
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {renderMemberModal()}

      {deleteTarget ? (
        <div className="settings-modal-backdrop" role="presentation">
          <div className="settings-modal project-delete-modal">
            <div className="project-create-head">
              <div>
                <div className="settings-eyebrow">Delete</div>
                <h3>Delete Team</h3>
                <p className="project-muted-copy">
                  Type <code>delete</code> to confirm deleting{" "}
                  <strong>{deleteTarget.label}</strong>.
                </p>
              </div>
            </div>
            <div className="settings-grid">
              <label className="settings-field">
                <span>Confirmation</span>
                <input
                  onChange={(event) => setDeleteConfirmationText(event.target.value)}
                  value={deleteConfirmationText}
                />
              </label>
            </div>
            <div className="settings-actions">
              <button
                className="secondary-btn"
                onClick={() => {
                  setDeleteTarget(null);
                  setDeleteConfirmationText("");
                }}
                type="button"
              >
                Cancel
              </button>
              <button
                className="primary-btn"
                disabled={busy || deleteConfirmationText !== "delete"}
                onClick={() => void deleteTeam()}
                type="button"
              >
                Delete Team
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </section>
  );
}
