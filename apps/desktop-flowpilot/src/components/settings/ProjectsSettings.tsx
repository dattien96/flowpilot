import { useEffect, useMemo, useState } from "react";
import {
  assertValidProjectBindings,
  resolveProviderKeyForModel,
  type Integration,
  type LocalRunnerArtifact,
  type Project,
  type ProjectPlatform,
  type ProjectWorkspaceBinding,
  type SupportedModel,
  type Team,
  type TeamMember,
  type ArtifactRun,
  type WorkflowRun,
} from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { RUNNER_URL } from "@/config";
import { formatTimestamp, integrationTypes, toErrorMessage, validateDirectoryBindingsInOrder } from "@/components/settings/settingsHelpers";
import {
  autoInitProjectEngine,
} from "@/components/settings/projectEngine";
import { ScaffoldActivity } from "@/components/settings/ScaffoldActivity";

type BindingDraft = Pick<ProjectWorkspaceBinding, "id" | "localPath" | "label"> & { persisted: boolean };
type ProjectTargetSection = "teams" | "workflows" | "artifacts" | "google-drive" | "jira-mcp";
type ProjectPanelKey = "overview" | "bindings" | "teams" | "mcp" | "runs" | "artifacts" | "chatSync" | "canonicalHead";

interface ChatSyncGoogleDriveAccountStatus {
  accountId: string;
  accountEmail?: string;
  status: string;
  accountReady: boolean;
  mcpWriteReady: boolean;
}

interface ChatSyncGoogleDriveConnection {
  projectId: string;
  status: string;
  folderId?: string;
  folderName?: string;
  accountId?: string;
  accountEmail?: string;
  lastError?: string;
  lastValidatedAt?: string;
  connectedAt?: string;
  updatedAt?: string;
}

interface ChatSyncGoogleDriveSession {
  sessionId: string;
  projectId: string;
  status: string;
  connectUrl?: string;
  expiresAt: string;
  connectedAt?: string;
  accountId?: string;
  accountEmail?: string;
  folderId?: string;
  folderName?: string;
  lastError?: string;
}

interface ChatSyncGoogleDriveStatus {
  connection: ChatSyncGoogleDriveConnection;
  session?: ChatSyncGoogleDriveSession;
  effectiveSource: "chat_sync" | "artifact_legacy" | "none";
  ready: boolean;
  availableAccounts: ChatSyncGoogleDriveAccountStatus[];
}

interface ChatSyncGoogleDriveConnectSession {
  sessionId: string;
  connectUrl?: string;
  status: string;
}

// Canonical Head / Change Contract (Task-188, CP-43 P-5) — mirrors
// apps/local-runner's changecontract.CanonicalHead / changecontract.Contract
// JSON shapes served by canonical_head_handlers.go.
interface CanonicalHeadDecision {
  tried: string;
  outcome: "adopted" | "rejected" | "reverted";
  reason: string;
  superseded_by?: string;
  source_doc_id?: string;
  at?: string | null;
}

interface CanonicalHeadResult {
  feature_key: string;
  behavior_statement: string;
  intent_signature: string;
  status: string;
  spec_confidence: string;
  decisions: CanonicalHeadDecision[];
  found: boolean;
}

interface StepContractResult {
  run_id: string;
  step_id: string;
  feature_key: string;
  intent: string;
  declared_paths: string[];
  declared_symbols: string[];
  confidence: string;
  found: boolean;
  touched_paths?: string[];
  in_scope_paths?: string[];
  out_of_scope_paths?: string[];
  scope_diff_note?: string;
}

interface ProjectsSettingsProps {
  onNavigateSection?: (section: ProjectTargetSection) => void;
}

// Each option installs the common skill pack plus its same-named skill folder.
// "none" installs common skills only; "kmm" additionally pulls android + ios.
const PROJECT_PLATFORM_OPTIONS: ReadonlyArray<{ value: ProjectPlatform; label: string }> = [
  { value: "none", label: "none (common skills only)" },
  { value: "android", label: "android" },
  { value: "ios", label: "ios" },
  { value: "kmm", label: "kotlin multiplatform (kmm)" },
  { value: "react-native", label: "react native" },
  { value: "flutter", label: "flutter" },
  { value: "reactjs", label: "reactjs" },
  { value: "vuejs", label: "vuejs" },
  { value: "angularjs", label: "angularjs" },
  { value: "golang", label: "golang" },
  { value: "java", label: "java" },
  { value: "python", label: "python" },
  { value: "nodejs", label: "nodejs" },
];

function renderPlatformOptions(): React.ReactNode {
  return PROJECT_PLATFORM_OPTIONS.map((option) => (
    <option key={option.value} value={option.value}>{option.label}</option>
  ));
}

function createEmptyProjectForm() {
  return {
    name: "",
    description: "",
    platform: "none" as ProjectPlatform,
    repositoryUrl: "",
    status: "active",
    xcodeScheme: "",
    xcodeDestination: "",
  };
}

function createEmptyCreateForm() {
  return {
    name: "",
    description: "",
    platform: "none" as ProjectPlatform,
    repositoryUrl: "",
    directoryPath: "",
    xcodeScheme: "",
    xcodeDestination: "",
  };
}

function defaultExpandedPanels(): Record<ProjectPanelKey, boolean> {
  return {
    overview: true,
    bindings: false,
    teams: false,
    mcp: false,
    runs: false,
    artifacts: false,
    chatSync: false,
    canonicalHead: false,
  };
}

function runnerFetch(path: string, init?: RequestInit): Promise<Response> {
  return fetch(new URL(path, RUNNER_URL).toString(), { cache: "no-store", ...init });
}

async function readRunnerError(response: Response): Promise<string> {
  const text = await response.text().catch(() => "");
  if (!text) return `Request failed with status ${response.status}.`;
  try {
    const payload = JSON.parse(text) as { error?: { message?: string } | string };
    if (typeof payload.error === "string") return payload.error;
    return payload.error?.message || text;
  } catch {
    return text;
  }
}

function chooseMcpTarget(linkedIntegrationIds: Record<string, string>, integrations: Integration[]): ProjectTargetSection {
  if (linkedIntegrationIds.google_drive || integrations.some((integration) => integration.type === "google_drive")) {
    return "google-drive";
  }
  return "jira-mcp";
}

export function ProjectsSettings({ onNavigateSection }: ProjectsSettingsProps): React.ReactElement {
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [models, setModels] = useState<SupportedModel[]>([]);
  const [integrations, setIntegrations] = useState<Integration[]>([]);
  const [runs, setRuns] = useState<WorkflowRun[]>([]);
  const [artifactRuns, setArtifactRuns] = useState<ArtifactRun[]>([]);
  const [localArtifacts, setLocalArtifacts] = useState<LocalRunnerArtifact[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState("");
  const [selectedTeamIds, setSelectedTeamIds] = useState<string[]>([]);
  const [linkedIntegrationIds, setLinkedIntegrationIds] = useState<Record<string, string>>({});
  const [bindings, setBindings] = useState<BindingDraft[]>([]);
  const [projectForm, setProjectForm] = useState(createEmptyProjectForm);
  const [createForm, setCreateForm] = useState(createEmptyCreateForm);
  const [defaults, setDefaults] = useState({
    defaultModel: "gpt-5.4",
    defaultReasoningEffort: "medium",
    sessionIdleTtlMinutes: 120,
  });
  const [createSelectedTeamIds, setCreateSelectedTeamIds] = useState<string[]>([]);
  const [showCreateView, setShowCreateView] = useState(false);
  const [pickingCreateDirectory, setPickingCreateDirectory] = useState(false);
  const [expandedPanels, setExpandedPanels] = useState<Record<ProjectPanelKey, boolean>>(defaultExpandedPanels);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [deleteConfirmationText, setDeleteConfirmationText] = useState("");
  const [chatSyncStatus, setChatSyncStatus] = useState<ChatSyncGoogleDriveStatus | null>(null);
  const [chatSyncLoading, setChatSyncLoading] = useState(false);
  const [chatSyncBusyAction, setChatSyncBusyAction] = useState<string | null>(null);
  const [chatSyncSelectedAccountId, setChatSyncSelectedAccountId] = useState("");
  const [canonicalHeadFeatureKey, setCanonicalHeadFeatureKey] = useState("");
  const [canonicalHeadResult, setCanonicalHeadResult] = useState<CanonicalHeadResult | null>(null);
  const [canonicalHeadLoading, setCanonicalHeadLoading] = useState(false);
  const [canonicalHeadError, setCanonicalHeadError] = useState<string | null>(null);
  const [stepContractRunId, setStepContractRunId] = useState("");
  const [stepContractStepId, setStepContractStepId] = useState("");
  const [stepContractResult, setStepContractResult] = useState<StepContractResult | null>(null);
  const [stepContractLoading, setStepContractLoading] = useState(false);
  const [stepContractError, setStepContractError] = useState<string | null>(null);
  const [headActionBusy, setHeadActionBusy] = useState(false);
  const [retireAction, setRetireAction] = useState<"renamed" | "merged" | "deprecated">("renamed");
  const [retireTargets, setRetireTargets] = useState("");
  const [projectFeatureKeys, setProjectFeatureKeys] = useState<string[]>([]);

  const selectedProject = useMemo(
    () => projects.find((project) => project.id === selectedProjectId) ?? null,
    [projects, selectedProjectId],
  );
  const linkedTeams = useMemo(
    () => teams.filter((team) => selectedTeamIds.includes(team.id)),
    [selectedTeamIds, teams],
  );
  const visibleMembers = useMemo(
    () => members.filter((member) => selectedTeamIds.includes(member.teamId)),
    [members, selectedTeamIds],
  );
  const visibleRuns = useMemo(
    () => runs.filter((run) => run.projectId === selectedProjectId).slice(0, 8),
    [runs, selectedProjectId],
  );
  const visibleArtifactRuns = useMemo(
    () => artifactRuns.filter((artifact) => artifact.projectId === selectedProjectId).slice(0, 8),
    [artifactRuns, selectedProjectId],
  );
  const visibleLocalArtifacts = useMemo(
    () => localArtifacts.filter((artifact) => artifact.projectId === selectedProjectId).slice(0, 8),
    [localArtifacts, selectedProjectId],
  );

  useEffect(() => {
    setBindings([]);
  }, [selectedProjectId]);

  const refresh = async (nextProjectId?: string) => {
    setLoading(true);
    try {
      const admin = await getAdminUseCases();
      const [nextProjects, nextTeams, nextModels, nextIntegrations, nextRuns, nextArtifactRuns, nextLocalArtifacts] = await Promise.all([
        admin.projects.listProjects(),
        admin.teams.listTeams(),
        admin.providers.listSupportedModels(),
        admin.integrations.listIntegrations(),
        admin.workflows.listWorkflowRuns(),
        admin.artifacts.listRuns(),
        admin.artifacts.listLocalArtifacts(),
      ]);
      const nextMembers = (await Promise.all(nextTeams.map((team) => admin.teams.listMembers(team.id)))).flat();
      setProjects(nextProjects);
      setTeams(nextTeams);
      setMembers(nextMembers);
      setModels(nextModels);
      setIntegrations(nextIntegrations);
      setRuns(nextRuns);
      setArtifactRuns(nextArtifactRuns);
      setLocalArtifacts(nextLocalArtifacts);
      setSelectedProjectId(
        nextProjectId && nextProjects.some((project) => project.id === nextProjectId)
          ? nextProjectId
          : nextProjects[0]?.id ?? "",
      );
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to load project settings."));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!selectedProject) return;
    void (async () => {
      try {
        const admin = await getAdminUseCases();
        const [projectTeams, projectBindings, linked] = await Promise.all([
          admin.teams.listTeamsByProject(selectedProject.id),
          admin.projects.listBindings(selectedProject.id),
          admin.integrations.listLinkedIntegrations(selectedProject.id),
        ]);
        setSelectedTeamIds(projectTeams.map((team) => team.id));
        setBindings(projectBindings.map((binding) => ({ ...binding, persisted: true })));
        setLinkedIntegrationIds(Object.fromEntries(linked.map((integration) => [integration.type, integration.id])));
        setProjectForm({
          name: selectedProject.name,
          description: selectedProject.description,
          platform: selectedProject.platform,
          repositoryUrl: selectedProject.repositoryUrl,
          status: selectedProject.status,
          xcodeScheme: selectedProject.xcodeScheme ?? "",
          xcodeDestination: selectedProject.xcodeDestination ?? "",
        });
        setDefaults({
          defaultModel: selectedProject.defaultModel ?? models[0]?.modelId ?? "gpt-5.4",
          defaultReasoningEffort: selectedProject.defaultReasoningEffort ?? "medium",
          sessionIdleTtlMinutes: selectedProject.sessionIdleTtlMinutes ?? 120,
        });
      } catch (error) {
        setMessage(toErrorMessage(error, "Unable to load selected project."));
      }
    })();
  }, [models, selectedProject]);

  const loadChatSyncStatus = async (projectId: string, sessionId?: string) => {
    setChatSyncLoading(true);
    try {
      const suffix = sessionId ? `?sessionId=${encodeURIComponent(sessionId)}` : "";
      const response = await runnerFetch(`/client/projects/${encodeURIComponent(projectId)}/chat-sync/google-drive/status${suffix}`);
      if (!response.ok) {
        throw new Error(await readRunnerError(response));
      }
      const payload = (await response.json()) as ChatSyncGoogleDriveStatus;
      setChatSyncStatus(payload);
      const selectedAccountId = payload.connection.accountId?.trim() ?? "";
      if (selectedAccountId) {
        setChatSyncSelectedAccountId(selectedAccountId);
      } else if (payload.availableAccounts.length === 1) {
        setChatSyncSelectedAccountId(payload.availableAccounts[0]?.accountId ?? "");
      }
      return payload;
    } catch (error) {
      setChatSyncStatus(null);
      setMessage(toErrorMessage(error, "Unable to load chat sync folder status."));
      return null;
    } finally {
      setChatSyncLoading(false);
    }
  };

  // Task-188 (CP-43 P-5): admin Canonical Head lookup — feature keys from catalog,
  // ledger, and on-disk canonical heads (runner list endpoint).
  const loadProjectFeatureKeys = async () => {
    if (!selectedProject?.directoryPath) {
      setProjectFeatureKeys([]);
      return;
    }
    try {
      const params = new URLSearchParams({ workingDirectory: selectedProject.directoryPath });
      const response = await runnerFetch(
        `/client/projects/${encodeURIComponent(selectedProject.id)}/features?${params.toString()}`,
      );
      if (!response.ok) {
        setProjectFeatureKeys([]);
        return;
      }
      const body = (await response.json()) as { feature_keys?: string[] };
      setProjectFeatureKeys(body.feature_keys ?? []);
    } catch {
      setProjectFeatureKeys([]);
    }
  };

  const loadCanonicalHead = async () => {
    if (!selectedProject?.directoryPath || !canonicalHeadFeatureKey.trim()) {
      return;
    }
    setCanonicalHeadLoading(true);
    setCanonicalHeadError(null);
    try {
      const params = new URLSearchParams({ workingDirectory: selectedProject.directoryPath });
      const response = await runnerFetch(
        `/client/projects/${encodeURIComponent(selectedProject.id)}/features/${encodeURIComponent(canonicalHeadFeatureKey.trim())}/canonical-head?${params.toString()}`,
      );
      if (!response.ok) {
        throw new Error(await readRunnerError(response));
      }
      setCanonicalHeadResult((await response.json()) as CanonicalHeadResult);
    } catch (error) {
      setCanonicalHeadResult(null);
      setCanonicalHeadError(toErrorMessage(error, "Unable to load the Canonical Head."));
    } finally {
      setCanonicalHeadLoading(false);
    }
  };

  const loadStepContract = async () => {
    if (!stepContractRunId.trim() || !stepContractStepId.trim()) {
      return;
    }
    setStepContractLoading(true);
    setStepContractError(null);
    try {
      const response = await runnerFetch(
        `/client/workflow-runs/${encodeURIComponent(stepContractRunId.trim())}/steps/${encodeURIComponent(stepContractStepId.trim())}/contract`,
      );
      if (!response.ok) {
        throw new Error(await readRunnerError(response));
      }
      setStepContractResult((await response.json()) as StepContractResult);
    } catch (error) {
      setStepContractResult(null);
      setStepContractError(toErrorMessage(error, "Unable to load the step Contract."));
    } finally {
      setStepContractLoading(false);
    }
  };

  // Task-186 r-attach-spec: human confirms the newly attached spec matches
  // current behavior, flipping a spec_less Head to current.
  const rebaselineCanonicalHead = async () => {
    if (!selectedProject?.directoryPath || !canonicalHeadResult?.feature_key) {
      return;
    }
    setHeadActionBusy(true);
    setCanonicalHeadError(null);
    try {
      const response = await runnerFetch(
        `/client/projects/${encodeURIComponent(selectedProject.id)}/features/${encodeURIComponent(canonicalHeadResult.feature_key)}/canonical-head/rebaseline`,
        {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ workingDirectory: selectedProject.directoryPath }),
        },
      );
      if (!response.ok) {
        throw new Error(await readRunnerError(response));
      }
      setCanonicalHeadResult((await response.json()) as CanonicalHeadResult);
    } catch (error) {
      setCanonicalHeadError(toErrorMessage(error, "Unable to rebaseline the Canonical Head."));
    } finally {
      setHeadActionBusy(false);
    }
  };

  // Task-187 r-retire: human-initiated rename/merge/deprecate. Preserves the
  // retiring Head's decisions into the target(s) for rename/merge.
  const retireCanonicalHead = async () => {
    if (!selectedProject?.directoryPath || !canonicalHeadResult?.feature_key) {
      return;
    }
    const targets = retireTargets
      .split(",")
      .map((value) => value.trim())
      .filter((value) => value !== "");
    if ((retireAction === "renamed" || retireAction === "merged") && targets.length === 0) {
      setCanonicalHeadError("Rename/merge requires at least one target feature key.");
      return;
    }
    setHeadActionBusy(true);
    setCanonicalHeadError(null);
    try {
      const response = await runnerFetch(
        `/client/projects/${encodeURIComponent(selectedProject.id)}/features/${encodeURIComponent(canonicalHeadResult.feature_key)}/canonical-head/retire`,
        {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ workingDirectory: selectedProject.directoryPath, action: retireAction, targets }),
        },
      );
      if (!response.ok) {
        throw new Error(await readRunnerError(response));
      }
      setCanonicalHeadResult((await response.json()) as CanonicalHeadResult);
      setRetireTargets("");
    } catch (error) {
      setCanonicalHeadError(toErrorMessage(error, "Unable to retire the Canonical Head."));
    } finally {
      setHeadActionBusy(false);
    }
  };

  useEffect(() => {
    if (!selectedProjectId) {
      setChatSyncStatus(null);
      setChatSyncLoading(false);
      setChatSyncSelectedAccountId("");
      return;
    }
    void loadChatSyncStatus(selectedProjectId);
  }, [selectedProjectId]);

  useEffect(() => {
    void loadProjectFeatureKeys();
  }, [selectedProject?.id, selectedProject?.directoryPath]);

  useEffect(() => {
    const handleMessage = (event: MessageEvent) => {
      if (event.data?.type === "flowpilot-google-drive-connected" || event.data?.type === "flowpilot-google-drive-account-connected") {
        if (!selectedProjectId) return;
        void loadChatSyncStatus(selectedProjectId).then(() => {
          setChatSyncBusyAction(null);
        });
      }
    };
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [selectedProjectId]);

  const pollChatSyncStatusUntilSettled = async (projectId: string, sessionId: string) => {
    const maxAttempts = 80;
    for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
      await new Promise((resolve) => window.setTimeout(resolve, 1500));
      const status = await loadChatSyncStatus(projectId, sessionId);
      const sessionStatus = status?.session?.status ?? "";
      if (status?.ready || sessionStatus === "connected" || sessionStatus === "failed" || sessionStatus === "expired") {
        setChatSyncBusyAction(null);
        return;
      }
    }
    setChatSyncBusyAction(null);
  };

  const startChatSyncFolderSelection = async () => {
    if (!selectedProjectId) return;
    setChatSyncBusyAction("connect");
    setMessage(null);
    try {
      const response = await runnerFetch(`/client/projects/${encodeURIComponent(selectedProjectId)}/chat-sync/google-drive/connect-session`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(chatSyncSelectedAccountId ? { accountId: chatSyncSelectedAccountId } : {}),
      });
      if (!response.ok) {
        throw new Error(await readRunnerError(response));
      }
      const session = (await response.json()) as ChatSyncGoogleDriveConnectSession;
      if (typeof session.connectUrl === "string" && session.connectUrl.trim()) {
        window.open(session.connectUrl, "_blank", "width=980,height=820");
      }
      void pollChatSyncStatusUntilSettled(selectedProjectId, session.sessionId);
      setMessage("Chat sync folder selection started. Complete the popup to save the folder.");
    } catch (error) {
      setChatSyncBusyAction(null);
      setMessage(toErrorMessage(error, "Unable to start chat sync folder selection."));
    }
  };

  const createProject = async () => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      assertValidProjectBindings([{ localPath: createForm.directoryPath }]);
      const project = await admin.projects.createProject({
        ...createForm,
        directoryPath: createForm.directoryPath.trim(),
        status: "active",
        artifactStoragePreference: "supabase",
      });
      await admin.projects.saveBinding(project.id, {
        localPath: createForm.directoryPath.trim(),
        label: "Primary",
      });
      await admin.teams.setProjectTeams(project.id, createSelectedTeamIds);
      void autoInitProjectEngine(project.id, [{ localPath: createForm.directoryPath.trim() }], createForm.platform, { xcodeScheme: createForm.xcodeScheme, xcodeDestination: createForm.xcodeDestination });
      setCreateForm(createEmptyCreateForm());
      setCreateSelectedTeamIds([]);
      setShowCreateView(false);
      await refresh(project.id);
      setMessage("Project created.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to create project."));
    } finally {
      setBusy(false);
    }
  };

  const pickCreateProjectDirectory = async () => {
    setPickingCreateDirectory(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const selection = await admin.directories.pickDirectory();
      setCreateForm((current) => ({ ...current, directoryPath: selection.path }));
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to select a directory. Paste the path manually if needed."));
    } finally {
      setPickingCreateDirectory(false);
    }
  };

  const saveProjectSettings = async () => {
    if (!selectedProject) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const normalizedBindings = bindings
        .map((binding) => ({
          ...binding,
          localPath: binding.localPath.trim(),
          label: binding.label?.trim() ?? "",
        }))
        .filter((binding) => binding.localPath.length > 0);
      assertValidProjectBindings(normalizedBindings.map((binding) => ({ localPath: binding.localPath })));
      const provider = resolveProviderKeyForModel(defaults.defaultModel, models);
      if (!provider) throw new Error("Default model is not mapped to a provider.");
      await admin.projects.updateProject(selectedProject.id, {
        name: projectForm.name,
        description: projectForm.description,
        platform: projectForm.platform,
        repositoryUrl: projectForm.repositoryUrl,
        status: projectForm.status,
        directoryPath: normalizedBindings[0]?.localPath ?? null,
        defaultProvider: provider,
        defaultModel: defaults.defaultModel,
        defaultReasoningEffort: defaults.defaultReasoningEffort as Project["defaultReasoningEffort"],
        sessionIdleTtlMinutes: defaults.sessionIdleTtlMinutes,
        xcodeScheme: projectForm.xcodeScheme.trim() || null,
        xcodeDestination: projectForm.xcodeDestination.trim() || null,
      });
      await admin.teams.setProjectTeams(selectedProject.id, selectedTeamIds);
      for (const type of integrationTypes) {
        await admin.integrations.setProjectIntegration(selectedProject.id, type, linkedIntegrationIds[type] || null);
      }
      const persistedBindingIds = new Set(normalizedBindings.filter((binding) => binding.persisted).map((binding) => binding.id));
      const existingBindings = await admin.projects.listBindings(selectedProject.id);
      await Promise.all(
        existingBindings
          .filter((binding) => !persistedBindingIds.has(binding.id))
          .map((binding) => admin.projects.deleteBinding(binding.id)),
      );
      for (const [index, binding] of normalizedBindings.entries()) {
        await admin.projects.saveBinding(selectedProject.id, {
          id: binding.persisted ? binding.id : undefined,
          localPath: binding.localPath,
          label: binding.label || (index === 0 ? "Primary" : null),
        });
      }
      void autoInitProjectEngine(selectedProject.id, normalizedBindings, projectForm.platform, { xcodeScheme: projectForm.xcodeScheme, xcodeDestination: projectForm.xcodeDestination });
      await refresh(selectedProject.id);
      setMessage("Project settings saved.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save project settings."));
    } finally {
      setBusy(false);
    }
  };

  const validateBindings = async () => {
    try {
      const admin = await getAdminUseCases();
      const usablePath = await validateDirectoryBindingsInOrder(
        bindings.map((binding) => ({
          ...binding,
          projectId: selectedProjectId,
          createdAt: "",
          updatedAt: "",
        })),
        admin.directories,
      );
      setMessage(`Usable binding: ${usablePath}`);
    } catch (error) {
      setMessage(toErrorMessage(error, "No usable binding found."));
    }
  };

  const deleteProject = async () => {
    if (!selectedProject) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      await admin.projects.deleteProject(selectedProject.id);
      setDeleteConfirmationText("");
      setDeleteModalOpen(false);
      await refresh();
      setMessage("Project deleted.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to delete project."));
    } finally {
      setBusy(false);
    }
  };

  const togglePanel = (panel: ProjectPanelKey) => {
    setExpandedPanels((current) => ({ ...current, [panel]: !current[panel] }));
  };

  const openSection = (section: ProjectTargetSection) => {
    onNavigateSection?.(section);
  };

  const renderCollapsibleSection = (
    panel: ProjectPanelKey,
    title: string,
    description: string,
    action: React.ReactNode,
    content: React.ReactNode,
  ) => (
    <section className="settings-subpanel project-detail-panel">
      <div className="project-panel-head">
        <button className="project-panel-toggle" onClick={() => togglePanel(panel)} type="button">
          <span>
            <strong>{title}</strong>
            <small>{description}</small>
          </span>
          <span className="project-panel-chevron">{expandedPanels[panel] ? "v" : ">"}</span>
        </button>
        {action ? <div className="project-panel-head-actions">{action}</div> : null}
      </div>
      {expandedPanels[panel] ? <div className="project-panel-body">{content}</div> : null}
    </section>
  );

  return (
    <section className="settings-panel">
      <div className="settings-panel-head">
        <div>
          <div className="settings-eyebrow">Projects</div>
          <h2>Projects</h2>
          <p>Manage projects, directory bindings, teams, MCP links, defaults, and generated artifacts.</p>
        </div>
        <button
          aria-label="Create project"
          className="primary-btn project-create-fab"
          onClick={() => {
            setShowCreateView(true);
            setMessage(null);
          }}
          type="button"
        >
          +
        </button>
      </div>
      {message ? <div className="settings-feedback">{message}</div> : null}
      {loading ? <div className="settings-feedback">Loading projects...</div> : null}
      {/* CA-916: live AI scaffold transcript for the selected/created project —
          self-hides when the runner reports no scaffold activity. */}
      <ScaffoldActivity projectId={selectedProjectId} />

      {showCreateView ? (
        <div className="settings-subpanel">
          <div className="project-create-back-row">
            <button aria-label="Back to registry" className="secondary-btn project-icon-btn" onClick={() => setShowCreateView(false)} title="Back to registry" type="button">&lt;</button>
          </div>
          <div className="project-create-head">
            <div>
              <h3>Create Project</h3>
              <p className="project-muted-copy">Create lives in its own view inside this tab, instead of the project registry page.</p>
            </div>
          </div>
          <div className="settings-grid">
            <label className="settings-field"><span>Name</span><input value={createForm.name} onChange={(event) => setCreateForm((current) => ({ ...current, name: event.target.value }))} /></label>
            <label className="settings-field"><span>Repository URL</span><input value={createForm.repositoryUrl} onChange={(event) => setCreateForm((current) => ({ ...current, repositoryUrl: event.target.value }))} /></label>
            <label className="settings-field settings-field-full"><span>Description</span><textarea value={createForm.description} onChange={(event) => setCreateForm((current) => ({ ...current, description: event.target.value }))} /></label>
            <label className="settings-field"><span>Platform</span><select value={createForm.platform} onChange={(event) => setCreateForm((current) => ({ ...current, platform: event.target.value as ProjectPlatform, xcodeScheme: "", xcodeDestination: "" }))}>{renderPlatformOptions()}</select></label>
            {createForm.platform === "ios" && (<>
              <label className="settings-field"><span>Xcode Scheme</span><input placeholder="e.g. MyApp" value={createForm.xcodeScheme} onChange={(event) => setCreateForm((current) => ({ ...current, xcodeScheme: event.target.value }))} /></label>
              <label className="settings-field settings-field-full"><span>Xcode Destination</span><input placeholder="e.g. platform=iOS Simulator,name=iPhone 15" value={createForm.xcodeDestination} onChange={(event) => setCreateForm((current) => ({ ...current, xcodeDestination: event.target.value }))} /></label>
            </>)}
            <div className="settings-field settings-field-full">
              <span>Primary Directory</span>
              <div className="settings-inline-row">
                <input
                  value={createForm.directoryPath}
                  onChange={(event) => setCreateForm((current) => ({ ...current, directoryPath: event.target.value }))}
                />
                <button
                  className="secondary-btn"
                  disabled={busy || pickingCreateDirectory}
                  onClick={() => void pickCreateProjectDirectory()}
                  type="button"
                >
                  {pickingCreateDirectory ? "Opening..." : "Browse Folder"}
                </button>
              </div>
              <small className="settings-field-hint">
                Opens the native folder chooser on macOS or Windows through the local runner.
              </small>
            </div>
          </div>
          <div className="settings-subpanel project-create-team-picker">
            <h3>Linked Teams</h3>
            <div className="settings-checkbox-list">
              {teams.length === 0 ? <div className="settings-empty">No teams exist yet.</div> : teams.map((team) => (
                <label className="settings-checkbox" key={team.id}><input checked={createSelectedTeamIds.includes(team.id)} onChange={() => setCreateSelectedTeamIds((current) => current.includes(team.id) ? current.filter((id) => id !== team.id) : [...current, team.id])} type="checkbox" /><span>{team.name}</span></label>
              ))}
            </div>
          </div>
          <div className="settings-actions"><button className="primary-btn" disabled={busy} onClick={() => void createProject()} type="button">Create Project</button></div>
        </div>
      ) : (
        <div className="project-layout">
          <aside className="settings-subpanel project-registry-panel">
            <div className="project-registry-head">
              <div>
                <h3>Project Registry</h3>
                <p className="project-muted-copy">Select a project from the list to inspect and edit its detail panels.</p>
              </div>
              <div className="project-count-badge">{projects.length}</div>
            </div>
            <div className="settings-list">
              {projects.length === 0 ? <div className="settings-empty">No projects found. Use the + button to create one.</div> : projects.map((project) => (
                <button className={`settings-list-item ${project.id === selectedProjectId ? "active" : ""}`} key={project.id} onClick={() => setSelectedProjectId(project.id)} type="button">
                  <div>
                    <strong>{project.name}</strong>
                    <span>{project.platform} / {project.status}</span>
                  </div>
                  <span>{formatTimestamp(project.updatedAt)}</span>
                </button>
              ))}
            </div>
          </aside>

          <div className="project-detail-column">
            {selectedProject ? (
              <>
                <div className="settings-subpanel project-detail-hero">
                  <div>
                    <h3>{selectedProject.name}</h3>
                    <p className="project-muted-copy">{selectedProject.description || "No project description yet."}</p>
                  </div>
                  <div className="settings-actions">
                    <button className="secondary-btn" onClick={() => setDeleteModalOpen(true)} type="button">Delete Project</button>
                    <button className="primary-btn" disabled={busy} onClick={() => void saveProjectSettings()} type="button">Save Project</button>
                  </div>
                </div>

                {renderCollapsibleSection(
                  "overview",
                  "Overview",
                  "Core project fields, AI defaults, and runtime session policy.",
                  null,
                  <div className="settings-grid">
                    <label className="settings-field"><span>Name</span><input value={projectForm.name} onChange={(event) => setProjectForm((current) => ({ ...current, name: event.target.value }))} /></label>
                    <label className="settings-field"><span>Repository URL</span><input value={projectForm.repositoryUrl} onChange={(event) => setProjectForm((current) => ({ ...current, repositoryUrl: event.target.value }))} /></label>
                    <label className="settings-field settings-field-full"><span>Description</span><textarea value={projectForm.description} onChange={(event) => setProjectForm((current) => ({ ...current, description: event.target.value }))} /></label>
                    <label className="settings-field"><span>Platform</span><select value={projectForm.platform} onChange={(event) => setProjectForm((current) => ({ ...current, platform: event.target.value as ProjectPlatform, xcodeScheme: "", xcodeDestination: "" }))}>{renderPlatformOptions()}</select></label>
                    {projectForm.platform === "ios" && (<>
                      <label className="settings-field"><span>Xcode Scheme</span><input placeholder="e.g. MyApp" value={projectForm.xcodeScheme} onChange={(event) => setProjectForm((current) => ({ ...current, xcodeScheme: event.target.value }))} /></label>
                      <label className="settings-field settings-field-full"><span>Xcode Destination</span><input placeholder="e.g. platform=iOS Simulator,name=iPhone 15" value={projectForm.xcodeDestination} onChange={(event) => setProjectForm((current) => ({ ...current, xcodeDestination: event.target.value }))} /></label>
                    </>)}
                    <label className="settings-field"><span>Status</span><select value={projectForm.status} onChange={(event) => setProjectForm((current) => ({ ...current, status: event.target.value }))}><option value="active">active</option><option value="archived">archived</option></select></label>
                    <label className="settings-field"><span>Default Model</span><select value={defaults.defaultModel} onChange={(event) => setDefaults((current) => ({ ...current, defaultModel: event.target.value }))}>{models.map((model) => <option key={model.id} value={model.modelId}>{model.displayName}</option>)}</select></label>
                    <label className="settings-field"><span>Reasoning</span><select value={defaults.defaultReasoningEffort} onChange={(event) => setDefaults((current) => ({ ...current, defaultReasoningEffort: event.target.value }))}><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="xhigh">Extra High</option></select></label>
                    <label className="settings-field"><span>Provider Session Idle TTL</span><input min={1} type="number" value={defaults.sessionIdleTtlMinutes} onChange={(event) => setDefaults((current) => ({ ...current, sessionIdleTtlMinutes: Number(event.target.value) || 1 }))} /></label>
                  </div>,
                )}

                {renderCollapsibleSection(
                  "bindings",
                  "Directory Bindings",
                  "The runner tries these bound local folders in order.",
                  <div className="settings-inline-actions">
                    <button className="secondary-btn" onClick={() => setBindings((current) => [...current, { id: crypto.randomUUID(), localPath: "", label: "", persisted: false }])} type="button">Add Binding</button>
                    <button className="secondary-btn" onClick={() => void validateBindings()} type="button">Validate Fallback</button>
                  </div>,
                  <>
                    <div className="settings-list">
                      {bindings.map((binding, index) => (
                        <div className="settings-card" key={binding.id}>
                          <div className="settings-card-head"><strong>{index === 0 ? "Primary" : `Binding ${index + 1}`}</strong><span>{binding.persisted ? "saved" : "new"}</span></div>
                          <div className="settings-grid">
                            <label className="settings-field"><span>Path</span><input value={binding.localPath} onChange={(event) => setBindings((current) => current.map((item) => item.id === binding.id ? { ...item, localPath: event.target.value } : item))} /></label>
                            <label className="settings-field"><span>Label</span><input value={binding.label ?? ""} onChange={(event) => setBindings((current) => current.map((item) => item.id === binding.id ? { ...item, label: event.target.value } : item))} /></label>
                          </div>
                          <div className="settings-actions"><button className="ghost-btn" disabled={bindings.length === 1 || busy} onClick={() => setBindings((current) => current.filter((item) => item.id !== binding.id))} type="button">Remove</button></div>
                        </div>
                      ))}
                    </div>
                  </>,
                )}

                {renderCollapsibleSection(
                  "teams",
                  "Teams",
                  "Linked teams and member snapshot for this project.",
                  <button aria-label="Open Teams tab" className="secondary-btn project-icon-btn" onClick={() => openSection("teams")} title="Open Teams tab" type="button">-&gt;</button>,
                  <>
                    <div className="settings-checkbox-list">
                      {teams.map((team) => (
                        <label className="settings-checkbox" key={team.id}><input checked={selectedTeamIds.includes(team.id)} onChange={() => setSelectedTeamIds((current) => current.includes(team.id) ? current.filter((id) => id !== team.id) : [...current, team.id])} type="checkbox" /><span>{team.name}</span></label>
                      ))}
                    </div>
                    <div className="project-inline-list">
                      <div className="project-inline-card">
                        <strong>Linked Teams</strong>
                        {linkedTeams.length === 0 ? <div className="settings-empty">No teams linked.</div> : linkedTeams.map((team) => <div className="project-inline-row" key={team.id}><span>{team.name}</span><span>{members.filter((member) => member.teamId === team.id).length} members</span></div>)}
                      </div>
                      <div className="project-inline-card">
                        <strong>Member Snapshot</strong>
                        {visibleMembers.length === 0 ? <div className="settings-empty">No member data available.</div> : visibleMembers.slice(0, 8).map((member) => <div className="project-inline-row" key={member.id}><span>{member.name}</span><span>{member.role} / {member.levelLabel}</span></div>)}
                      </div>
                    </div>
                  </>,
                )}

                {renderCollapsibleSection(
                  "mcp",
                  "MCP",
                  "Link this project to existing reusable MCP integrations.",
                  <button aria-label="Open MCP tab" className="secondary-btn project-icon-btn" onClick={() => openSection(chooseMcpTarget(linkedIntegrationIds, integrations))} title="Open MCP tab" type="button">-&gt;</button>,
                  <div className="settings-grid">
                    {integrationTypes.map((type) => (
                      <label className="settings-field" key={type}><span>{type.toUpperCase()}</span><select value={linkedIntegrationIds[type] ?? ""} onChange={(event) => setLinkedIntegrationIds((current) => ({ ...current, [type]: event.target.value }))}><option value="">Not linked</option>{integrations.filter((integration) => integration.type === type).map((integration) => <option key={integration.id} value={integration.id}>{integration.label}</option>)}</select></label>
                    ))}
                  </div>,
                )}

                {renderCollapsibleSection(
                  "runs",
                  "Run History",
                  "Recent workflow executions for this project.",
                  <button aria-label="Open Workflows tab" className="secondary-btn project-icon-btn" onClick={() => openSection("workflows")} title="Open Workflows tab" type="button">-&gt;</button>,
                  <div className="settings-list">
                    {visibleRuns.length === 0 ? <div className="settings-empty">No workflow runs yet.</div> : visibleRuns.map((run) => <div className="settings-list-item static" key={run.id}><strong>{run.status}</strong><span>{run.model ?? "model"} / {formatTimestamp(run.startedAt)}</span></div>)}
                  </div>,
                )}

                {renderCollapsibleSection(
                  "artifacts",
                  "Artifacts",
                  "Generated artifacts and synced artifact records for this project.",
                  <button aria-label="Open Artifacts tab" className="secondary-btn project-icon-btn" onClick={() => openSection("artifacts")} title="Open Artifacts tab" type="button">-&gt;</button>,
                  <div className="project-inline-list">
                    <div className="project-inline-card">
                      <strong>Generated Locally</strong>
                      {visibleLocalArtifacts.length === 0 ? <div className="settings-empty">No local artifacts yet.</div> : visibleLocalArtifacts.map((artifact) => <div className="project-inline-row" key={artifact.artifactId}><span>{artifact.title}</span><span>{artifact.syncStatus}</span></div>)}
                    </div>
                    <div className="project-inline-card">
                      <strong>Artifact Runs</strong>
                      {visibleArtifactRuns.length === 0 ? <div className="settings-empty">No artifact runs yet.</div> : visibleArtifactRuns.map((artifact) => <div className="project-inline-row" key={artifact.id}><span>{artifact.title}</span><span>{artifact.storageProvider ?? "local"} / {artifact.syncStatus}</span></div>)}
                    </div>
                  </div>,
                )}

                {renderCollapsibleSection(
                  "chatSync",
                  "Chat Sync",
                  "Project-level Google Drive folder used for remote chat sync and restore.",
                  <div className="settings-inline-actions">
                    <button className="secondary-btn" disabled={chatSyncLoading || chatSyncBusyAction !== null} onClick={() => void loadChatSyncStatus(selectedProject.id)} type="button">
                      {chatSyncLoading ? "Refreshing..." : "Refresh Status"}
                    </button>
                    <button className="secondary-btn" disabled={chatSyncBusyAction !== null} onClick={() => void startChatSyncFolderSelection()} type="button">
                      {chatSyncBusyAction === "connect" ? "Opening..." : "Select Drive Folder"}
                    </button>
                    {(chatSyncStatus?.availableAccounts.filter((account) => account.accountReady && account.mcpWriteReady).length ?? 0) === 0 ? (
                      <button className="secondary-btn" onClick={() => openSection("google-drive")} type="button">Open Google Drive Setup</button>
                    ) : null}
                  </div>,
                  <div className="project-inline-list">
                    <div className="project-inline-card">
                      <strong>Current Binding</strong>
                      {chatSyncStatus ? (
                        <>
                          <label className="settings-field">
                            <span>Selected Google Account</span>
                            <select
                              disabled={chatSyncBusyAction !== null || chatSyncLoading || chatSyncStatus.availableAccounts.length === 0}
                              value={chatSyncSelectedAccountId}
                              onChange={(event) => setChatSyncSelectedAccountId(event.target.value)}
                            >
                              <option value="">Auto pick connected account</option>
                              {chatSyncStatus.availableAccounts.map((account) => (
                                <option key={account.accountId} value={account.accountId}>
                                  {account.accountEmail || account.accountId} {account.mcpWriteReady ? "" : "(missing drive.file)"}
                                </option>
                              ))}
                            </select>
                          </label>
                          <div className="project-inline-row"><span>Effective source</span><span>{chatSyncStatus.effectiveSource}</span></div>
                          <div className="project-inline-row"><span>Status</span><span>{chatSyncStatus.connection.status}</span></div>
                          <div className="project-inline-row"><span>Account</span><span>{chatSyncStatus.connection.accountEmail || "—"}</span></div>
                          <div className="project-inline-row"><span>Folder</span><span>{chatSyncStatus.connection.folderName || "—"}</span></div>
                          <div className="project-inline-row"><span>Folder ID</span><span>{chatSyncStatus.connection.folderId || "—"}</span></div>
                          <div className="project-inline-row"><span>Last validated</span><span>{formatTimestamp(chatSyncStatus.connection.lastValidatedAt)}</span></div>
                          {chatSyncStatus.connection.lastError ? <div className="settings-feedback error">{chatSyncStatus.connection.lastError}</div> : null}
                        </>
                      ) : (
                        <div className="settings-empty">{chatSyncLoading ? "Loading chat sync folder status..." : "No chat sync folder selected yet."}</div>
                      )}
                    </div>
                    <div className="project-inline-card">
                      <strong>Connected Google Accounts</strong>
                      {chatSyncStatus?.availableAccounts.length ? (
                        chatSyncStatus.availableAccounts.map((account) => (
                          <div className="project-inline-row" key={account.accountId}>
                            <span>{account.accountEmail || account.accountId}</span>
                            <span>{account.mcpWriteReady ? account.status : "missing drive.file"}</span>
                          </div>
                        ))
                      ) : (
                        <div className="settings-empty">No Google Drive account is connected on this runner.</div>
                      )}
                    </div>
                  </div>,
                )}

                {renderCollapsibleSection(
                  "canonicalHead",
                  "Canonical Head",
                  "Look up a feature's Canonical Head (behavior, signature status, rejected decisions) and a step's declared Change Contract.",
                  null,
                  <div className="project-inline-list">
                    <div className="project-inline-card">
                      <strong>Feature</strong>
                      <div className="settings-inline-row">
                        <input
                          list="project-feature-keys"
                          onChange={(event) => setCanonicalHeadFeatureKey(event.target.value)}
                          placeholder="feature_key, e.g. calc-core"
                          value={canonicalHeadFeatureKey}
                        />
                        <datalist id="project-feature-keys">
                          {projectFeatureKeys.map((key) => (
                            <option key={key} value={key} />
                          ))}
                        </datalist>
                        <button className="secondary-btn" disabled={canonicalHeadLoading || !canonicalHeadFeatureKey.trim() || !selectedProject?.directoryPath} onClick={() => void loadCanonicalHead()} type="button">
                          {canonicalHeadLoading ? "Loading..." : "Look up"}
                        </button>
                      </div>
                      {!selectedProject?.directoryPath ? <div className="settings-empty">This project has no directory bound yet.</div> : null}
                      {canonicalHeadError ? <div className="settings-feedback error">{canonicalHeadError}</div> : null}
                      {canonicalHeadResult ? (
                        canonicalHeadResult.found ? (
                          <>
                            <div className="project-inline-row"><span>Status</span><span>{canonicalHeadResult.status}{canonicalHeadResult.spec_confidence === "spec_less" ? " (spec-less — low confidence)" : ""}</span></div>
                            <div className="project-inline-row"><span>Signature</span><span>{canonicalHeadResult.intent_signature.slice(0, 12)}</span></div>
                            <div className="project-inline-row"><span>Behavior</span><span>{canonicalHeadResult.behavior_statement || "(none recorded)"}</span></div>
                            {canonicalHeadResult.decisions.filter((d) => d.outcome !== "adopted").length > 0 ? (
                              <>
                                <div className="project-inline-row"><span>Do NOT re-attempt</span><span /></div>
                                {canonicalHeadResult.decisions.filter((d) => d.outcome !== "adopted").map((d, index) => (
                                  <div className="project-inline-row" key={`${d.tried}-${index}`}>
                                    <span>{d.tried}</span>
                                    <span>{d.reason && d.reason !== d.tried ? d.reason : ""}</span>
                                  </div>
                                ))}
                              </>
                            ) : null}
                            {canonicalHeadResult.spec_confidence === "spec_less" ? (
                              <div className="settings-inline-actions">
                                <button className="secondary-btn" disabled={headActionBusy} onClick={() => void rebaselineCanonicalHead()} type="button">
                                  {headActionBusy ? "Working..." : "Confirm spec & rebaseline"}
                                </button>
                                <small className="settings-field-hint">Attaches the feature's governing docs and flips the Head to current (r-attach-spec).</small>
                              </div>
                            ) : null}
                            <div className="settings-inline-row">
                              <select value={retireAction} onChange={(event) => setRetireAction(event.target.value as "renamed" | "merged" | "deprecated")}>
                                <option value="renamed">renamed</option>
                                <option value="merged">merged</option>
                                <option value="deprecated">deprecated</option>
                              </select>
                              {retireAction !== "deprecated" ? (
                                <input onChange={(event) => setRetireTargets(event.target.value)} placeholder="target feature keys (comma-separated)" value={retireTargets} />
                              ) : null}
                              <button className="ghost-btn" disabled={headActionBusy} onClick={() => void retireCanonicalHead()} type="button">
                                {headActionBusy ? "Working..." : "Retire feature"}
                              </button>
                            </div>
                            <small className="settings-field-hint">Retire preserves this Head's rejected decisions into the target(s) for rename/merge (r-retire); the file is kept for provenance.</small>
                          </>
                        ) : (
                          <div className="settings-empty">No Canonical Head found yet for this feature — it is minted on its first gate-passing turn.</div>
                        )
                      ) : null}
                    </div>
                    <div className="project-inline-card">
                      <strong>Step Contract</strong>
                      <div className="settings-inline-row">
                        <input onChange={(event) => setStepContractRunId(event.target.value)} placeholder="run id" value={stepContractRunId} />
                        <input onChange={(event) => setStepContractStepId(event.target.value)} placeholder="step id" value={stepContractStepId} />
                        <button className="secondary-btn" disabled={stepContractLoading || !stepContractRunId.trim() || !stepContractStepId.trim()} onClick={() => void loadStepContract()} type="button">
                          {stepContractLoading ? "Loading..." : "Look up"}
                        </button>
                      </div>
                      {stepContractError ? <div className="settings-feedback error">{stepContractError}</div> : null}
                      {stepContractResult ? (
                        stepContractResult.found ? (
                          <>
                            <div className="project-inline-row"><span>Feature key</span><span>{stepContractResult.feature_key || "(none)"}</span></div>
                            <div className="project-inline-row"><span>Confidence</span><span>{stepContractResult.confidence}</span></div>
                            <div className="project-inline-row"><span>Intent</span><span>{stepContractResult.intent || "(none declared)"}</span></div>
                            {stepContractResult.declared_paths.length > 0 ? (
                              <div className="project-inline-row"><span>Declared paths</span><span>{stepContractResult.declared_paths.join(", ")}</span></div>
                            ) : null}
                            {stepContractResult.scope_diff_note ? (
                              <div className="project-inline-row"><span>Scope diff</span><span>{stepContractResult.scope_diff_note}</span></div>
                            ) : null}
                            {stepContractResult.in_scope_paths && stepContractResult.in_scope_paths.length > 0 ? (
                              <div className="project-inline-row"><span>In scope (touched)</span><span>{stepContractResult.in_scope_paths.join(", ")}</span></div>
                            ) : null}
                            {stepContractResult.out_of_scope_paths && stepContractResult.out_of_scope_paths.length > 0 ? (
                              <div className="project-inline-row"><span>Out of scope</span><span className="settings-feedback error">{stepContractResult.out_of_scope_paths.join(", ")}</span></div>
                            ) : null}
                            {stepContractResult.touched_paths && stepContractResult.touched_paths.length > 0 && !stepContractResult.in_scope_paths?.length && !stepContractResult.out_of_scope_paths?.length ? (
                              <div className="project-inline-row"><span>Touched</span><span>{stepContractResult.touched_paths.join(", ")}</span></div>
                            ) : null}
                          </>
                        ) : (
                          <div className="settings-empty">No Contract found for that run/step.</div>
                        )
                      ) : null}
                    </div>
                  </div>,
                )}
              </>
            ) : (
              <div className="settings-subpanel">
                <div className="settings-empty">Select a project to inspect its detail panels.</div>
              </div>
            )}
          </div>
        </div>
      )}

      {deleteModalOpen && selectedProject ? (
        <div className="settings-modal-backdrop" role="presentation">
          <div className="settings-modal project-delete-modal">
            <div className="settings-panel-head">
              <div>
                <div className="settings-eyebrow">Delete Project</div>
                <h2>{selectedProject.name}</h2>
                <p>Type exactly <code>delete</code> to enable permanent deletion.</p>
              </div>
            </div>
            <div className="settings-grid">
              <label className="settings-field">
                <span>Confirmation</span>
                <input value={deleteConfirmationText} onChange={(event) => setDeleteConfirmationText(event.target.value)} />
              </label>
            </div>
            <div className="settings-actions">
              <button className="secondary-btn" onClick={() => { setDeleteModalOpen(false); setDeleteConfirmationText(""); }} type="button">Cancel</button>
              <button className="primary-btn" disabled={busy || deleteConfirmationText !== "delete"} onClick={() => void deleteProject()} type="button">Delete Project</button>
            </div>
          </div>
        </div>
      ) : null}
    </section>
  );
}
