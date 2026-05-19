import type { ContextSource } from "@/domain/model/entity/context-source";
import type { Feature } from "@/domain/model/entity/feature";
import type { Integration } from "@/domain/model/entity/integration";
import type { Project } from "@/domain/model/entity/project";
import type { Team, TeamMember } from "@/domain/model/entity/team";
import type {
  AiCallLog,
  AiOutput,
  Approval,
  ApprovalDecision,
  WorkflowDefinition,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow";

function createId(prefix: string) {
  return `${prefix}_${Math.random().toString(36).slice(2, 10)}`;
}

const now = new Date().toISOString();

export const demoProjects: Project[] = [
  {
    id: "project_meal_suggestion",
    name: "Meal Suggestion Android App",
    description: "Android product used to validate feature planning and widget flows.",
    platform: "android",
    repositoryUrl: "https://github.com/example/meal-suggestion",
    directoryPath: "/projects/meal-suggestion",
    ownerId: "demo-user",
    status: "active",
    artifactStoragePreference: "supabase",
    createdBy: "demo-user",
    createdAt: now,
    updatedAt: now,
  },
  {
    id: "project_flowpilot_admin",
    name: "FlowPilot Admin Web",
    description:
      "Internal admin surface for workflow orchestration, approvals, and engineering reports.",
    platform: "web",
    repositoryUrl: "https://github.com/example/flowpilot-admin",
    directoryPath: "/projects/flowpilot-admin",
    ownerId: "demo-user",
    status: "active",
    artifactStoragePreference: "supabase",
    createdBy: "demo-user",
    createdAt: now,
    updatedAt: now,
  },
];

export const demoTeams: Team[] = [];
export const demoTeamMembers: TeamMember[] = [];
export const demoProjectTeamLinks: Array<{ projectId: string; teamId: string }> = [];
export const demoProjectMcpLinks: Array<{
  projectId: string;
  integrationId: string;
  type: Integration["type"];
}> = [
  {
    projectId: "project_meal_suggestion",
    integrationId: "integration_meal_jira",
    type: "jira",
  },
  {
    projectId: "project_meal_suggestion",
    integrationId: "integration_meal_drive",
    type: "google_drive",
  },
  {
    projectId: "project_flowpilot_admin",
    integrationId: "integration_admin_firebase",
    type: "firebase",
  },
];
export const demoIntegrations: Integration[] = [
  {
    id: "integration_meal_jira",
    projectId: "project_meal_suggestion",
    type: "jira",
    label: "Meal Suggestion Jira",
    configEncrypted: {
      workspaceUrl: "https://jira.example.com/meal",
      board: "MEAL",
    },
    status: "connected",
    lastSyncedAt: now,
    lastError: null,
    createdAt: now,
    updatedAt: now,
  },
  {
    id: "integration_meal_drive",
    projectId: "project_meal_suggestion",
    type: "google_drive",
    label: "Meal Drive Folder",
    configEncrypted: {
      folderId: "drive-folder-meal",
    },
    status: "awaiting_oauth",
    lastSyncedAt: null,
    lastError: null,
    createdAt: now,
    updatedAt: now,
  },
  {
    id: "integration_admin_firebase",
    projectId: "project_flowpilot_admin",
    type: "firebase",
    label: "Admin Firebase",
    configEncrypted: {
      projectId: "flowpilot-admin",
      environment: "staging",
    },
    status: "failed",
    lastSyncedAt: null,
    lastError: "firebase-tools login is required on the runner host.",
    createdAt: now,
    updatedAt: now,
  },
];

export const demoFeatures: Feature[] = [
  {
    id: "feature_widget_refresh",
    projectId: "project_meal_suggestion",
    title: "Home Widget Meal Card Refresh",
    businessGoal: "Increase home screen engagement with glanceable meal content.",
    userProblem: "Users want a fresh meal suggestion without opening the app.",
    expectedFlow:
      "Widget displays meal image, title, and type. Refresh swaps the content in place.",
    acceptanceCriteria:
      "Refresh updates content, loading is graceful, and analytics capture interaction.",
    priority: "high",
    status: "active",
    ownerId: "demo-user",
    createdAt: now,
    updatedAt: now,
  },
  {
    id: "feature_release_readiness",
    projectId: "project_meal_suggestion",
    title: "Release Readiness Report",
    businessGoal: "Reduce risky Android releases with a structured AI-assisted preflight report.",
    userProblem: "Tech leads spend too much time manually stitching risk notes before release.",
    expectedFlow:
      "Owner selects the feature branch, test signals, and release notes to generate a decision-ready summary.",
    acceptanceCriteria:
      "Report covers blockers, rollout plan, analytics checks, and unresolved defects.",
    priority: "medium",
    status: "active",
    ownerId: "demo-user",
    createdAt: now,
    updatedAt: now,
  },
  {
    id: "feature_admin_approval_center",
    projectId: "project_flowpilot_admin",
    title: "Approval Center Queue",
    businessGoal: "Make pending workflow approvals visible in one place.",
    userProblem: "The owner loses track of workflow pauses spread across multiple features and runs.",
    expectedFlow:
      "Approval queue highlights which output is blocked, why it matters, and what happens after approval.",
    acceptanceCriteria:
      "Queue can deep link to the related run and clearly show approval urgency.",
    priority: "high",
    status: "active",
    ownerId: "demo-user",
    createdAt: now,
    updatedAt: now,
  },
];

export const demoContextSources: ContextSource[] = [
  {
    id: "context_widget_goals",
    projectId: "project_meal_suggestion",
    featureId: "feature_widget_refresh",
    type: "manual_text",
    title: "Business and UX notes",
    rawContent:
      "Widget needs common Android UX, a visible refresh control, and a low-friction loading state.",
    summarizedContent: null,
    createdBy: "demo-user",
    createdAt: now,
  },
  {
    id: "context_release_rules",
    projectId: "project_meal_suggestion",
    featureId: "feature_release_readiness",
    type: "manual_text",
    title: "Release guardrails",
    rawContent:
      "Include crash history, analytics schema changes, staged rollout notes, and customer support risk.",
    summarizedContent: null,
    createdBy: "demo-user",
    createdAt: now,
  },
  {
    id: "context_admin_review",
    projectId: "project_flowpilot_admin",
    featureId: "feature_admin_approval_center",
    type: "manual_text",
    title: "Approval center UX notes",
    rawContent:
      "Queue should prioritize waiting approvals, surface step names, and keep decision actions close to output context.",
    summarizedContent: null,
    createdBy: "demo-user",
    createdAt: now,
  },
  {
    id: "context_admin_scope",
    projectId: "project_flowpilot_admin",
    featureId: "feature_admin_approval_center",
    type: "url",
    title: "Planning reference",
    rawContent: "Google Doc reference for Admin MVP Skeleton and approval model.",
    summarizedContent: null,
    createdBy: "demo-user",
    createdAt: now,
  },
];

export const demoWorkflowDefinitions: WorkflowDefinition[] = [
  {
    id: "workflow_feature_to_android_tech_spec",
    name: "feature_to_android_tech_spec",
    description:
      "Convert a feature intake into business summary, product spec, Android tech spec, task breakdown, test plan, and risk report.",
    version: 1,
    status: "active",
    steps: [
      { key: "collect_context", name: "Collect Context", type: "tool" },
      {
        key: "generate_business_summary",
        name: "Generate Business Summary",
        type: "ai_mock",
        outputType: "business_summary",
      },
      { key: "approval_business_summary", name: "Approve Business Summary", type: "approval" },
      {
        key: "generate_product_spec",
        name: "Generate Product Spec",
        type: "ai_mock",
        outputType: "product_spec",
      },
      { key: "approval_product_spec", name: "Approve Product Spec", type: "approval" },
      {
        key: "generate_android_tech_spec",
        name: "Generate Android Tech Spec",
        type: "ai_mock",
        outputType: "android_tech_spec",
      },
      { key: "approval_android_tech_spec", name: "Approve Android Tech Spec", type: "approval" },
      {
        key: "generate_task_breakdown",
        name: "Generate Task Breakdown",
        type: "ai_mock",
        outputType: "task_breakdown",
      },
      {
        key: "generate_test_plan",
        name: "Generate Test Plan",
        type: "ai_mock",
        outputType: "test_plan",
      },
      {
        key: "generate_risk_report",
        name: "Generate Risk Report",
        type: "ai_mock",
        outputType: "risk_report",
      },
    ],
  },
];

export const demoWorkflowRuns: WorkflowRun[] = [];
export const demoWorkflowSteps: WorkflowStep[] = [];
export const demoOutputs: AiOutput[] = [];
export const demoApprovals: Approval[] = [];
export const demoApprovalDecisions: ApprovalDecision[] = [];
export const demoLogs: AiCallLog[] = [];

function createTimestamp(minutesAgo: number) {
  return new Date(Date.now() - minutesAgo * 60_000).toISOString();
}

export function buildWorkflowRun(
  featureId: string,
  projectId: string,
  workflowDefinitionId: string,
  contextSourceIds: string[],
): WorkflowRun {
  return {
    id: createId("run"),
    workflowDefinitionId,
    projectId,
    featureId,
    status: "running",
    currentStepKey: null,
    selectedContextSourceIds: contextSourceIds,
    startedBy: "demo-user",
    startedAt: new Date().toISOString(),
    completedAt: null,
    errorSummary: null,
  };
}

export function buildWorkflowSteps(
  runId: string,
  definition: WorkflowDefinition,
): WorkflowStep[] {
  return definition.steps.map((step, index) => ({
    id: createId("step"),
    workflowRunId: runId,
    stepKey: step.key,
    stepName: step.name,
    stepType: step.type,
    status: "pending",
    sequenceIndex: index,
    outputId: null,
    startedAt: null,
    completedAt: null,
    errorMessage: null,
  }));
}

function seedCompletedRun() {
  const runId = "run_seed_completed";
  const definition = demoWorkflowDefinitions[0];
  const steps = buildWorkflowSteps(runId, definition);

  const startedAt = createTimestamp(220);

  demoWorkflowRuns.push({
    id: runId,
    workflowDefinitionId: definition.id,
    projectId: "project_meal_suggestion",
    featureId: "feature_widget_refresh",
    status: "completed",
    currentStepKey: null,
    selectedContextSourceIds: ["context_widget_goals"],
    startedBy: "demo-user",
    startedAt,
    completedAt: createTimestamp(170),
    errorSummary: null,
  });

  for (const step of steps) {
    step.status = "completed";
    step.startedAt = startedAt;
    step.completedAt = createTimestamp(190 - step.sequenceIndex * 2);

    if (step.stepType === "ai_mock") {
      const outputId = `output_${step.id}`;
      step.outputId = outputId;

      demoOutputs.push({
        id: outputId,
        workflowRunId: runId,
        workflowStepId: step.id,
        projectId: "project_meal_suggestion",
        featureId: "feature_widget_refresh",
        outputType: step.stepKey.replace("generate_", "") as AiOutput["outputType"],
        version: 1,
        title: step.stepName,
        contentMarkdown:
          `# ${step.stepName}\n\nThis seeded output demonstrates a completed workflow artifact for the Android widget initiative.\n\n- Feature: Home Widget Meal Card Refresh\n- Focus: repeatable approvals and stored outputs\n- State: completed seed run`,
        isApproved: true,
        createdAt: createTimestamp(200 - step.sequenceIndex * 2),
      });

      demoLogs.push({
        id: `log_${step.id}`,
        workflowRunId: runId,
        workflowStepId: step.id,
        provider: "mock",
        model: "mock-planner-v1",
        inputTokens: 260 + step.sequenceIndex * 14,
        outputTokens: 390 + step.sequenceIndex * 20,
        costEstimate: Number((0.0022 + step.sequenceIndex * 0.0003).toFixed(4)),
        latencyMs: 180 + step.sequenceIndex * 24,
        status: "success",
        createdAt: createTimestamp(200 - step.sequenceIndex * 2),
      });
    }

    if (step.stepType === "approval") {
      demoApprovals.push({
        id: `approval_${step.id}`,
        workflowRunId: runId,
        workflowStepId: step.id,
        aiOutputId: demoOutputs.at(-1)?.id ?? null,
        status: "approved",
        reviewerId: "demo-user",
        comment: "Approved during seeded demo setup.",
        decidedAt: createTimestamp(195 - step.sequenceIndex * 2),
        createdAt: createTimestamp(196 - step.sequenceIndex * 2),
      });
    }
  }

  demoWorkflowSteps.push(...steps);
}

function seedWaitingApprovalRun() {
  const runId = "run_seed_waiting_approval";
  const definition = demoWorkflowDefinitions[0];
  const steps = buildWorkflowSteps(runId, definition);

  demoWorkflowRuns.push({
    id: runId,
    workflowDefinitionId: definition.id,
    projectId: "project_flowpilot_admin",
    featureId: "feature_admin_approval_center",
    status: "waiting_approval",
    currentStepKey: "approval_business_summary",
    selectedContextSourceIds: ["context_admin_review", "context_admin_scope"],
    startedBy: "demo-user",
    startedAt: createTimestamp(95),
    completedAt: null,
    errorSummary: null,
  });

  for (const step of steps) {
    if (step.stepKey === "collect_context") {
      step.status = "completed";
      step.startedAt = createTimestamp(95);
      step.completedAt = createTimestamp(94);
    } else if (step.stepKey === "generate_business_summary") {
      step.status = "completed";
      step.startedAt = createTimestamp(93);
      step.completedAt = createTimestamp(91);
      step.outputId = `output_${step.id}`;

      demoOutputs.push({
        id: step.outputId,
        workflowRunId: runId,
        workflowStepId: step.id,
        projectId: "project_flowpilot_admin",
        featureId: "feature_admin_approval_center",
        outputType: "business_summary",
        version: 1,
        title: "Approval Center Business Summary",
        contentMarkdown:
          "# Business Summary\n\nThe approval center should reduce context switching by consolidating all waiting outputs into one operator-focused queue.",
        isApproved: false,
        createdAt: createTimestamp(91),
      });

      demoLogs.push({
        id: `log_${step.id}`,
        workflowRunId: runId,
        workflowStepId: step.id,
        provider: "mock",
        model: "mock-planner-v1",
        inputTokens: 304,
        outputTokens: 488,
        costEstimate: 0.0028,
        latencyMs: 260,
        status: "success",
        createdAt: createTimestamp(91),
      });
    } else if (step.stepKey === "approval_business_summary") {
      step.status = "waiting_approval";
      step.startedAt = createTimestamp(90);

      demoApprovals.push({
        id: `approval_${step.id}`,
        workflowRunId: runId,
        workflowStepId: step.id,
        aiOutputId: demoOutputs.at(-1)?.id ?? null,
        status: "pending",
        reviewerId: null,
        comment: null,
        decidedAt: null,
        createdAt: createTimestamp(90),
      });
    }
  }

  demoWorkflowSteps.push(...steps);
}

function seedRunningRun() {
  const runId = "run_seed_running";
  const definition = demoWorkflowDefinitions[0];
  const steps = buildWorkflowSteps(runId, definition);

  demoWorkflowRuns.push({
    id: runId,
    workflowDefinitionId: definition.id,
    projectId: "project_meal_suggestion",
    featureId: "feature_release_readiness",
    status: "running",
    currentStepKey: "generate_business_summary",
    selectedContextSourceIds: ["context_release_rules"],
    startedBy: "demo-user",
    startedAt: createTimestamp(24),
    completedAt: null,
    errorSummary: null,
  });

  for (const step of steps) {
    if (step.stepKey === "collect_context") {
      step.status = "completed";
      step.startedAt = createTimestamp(24);
      step.completedAt = createTimestamp(23);
    } else if (step.stepKey === "generate_business_summary") {
      step.status = "running";
      step.startedAt = createTimestamp(22);
    }
  }

  demoWorkflowSteps.push(...steps);
}

seedCompletedRun();
seedWaitingApprovalRun();
seedRunningRun();
