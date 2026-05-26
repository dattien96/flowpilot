import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type {
  ArtifactDefinition,
  ArtifactRun,
  StepDefinition,
  SupportedStepModel,
  Workflow,
  WorkflowRunStartRequest,
  WorkflowRun,
  WorkflowRunLog,
  WorkflowRunStep,
  WorkflowStep,
  StepType,
} from "@/domain/model/entity/workflow-engine";
import { deriveStepPromptBase } from "@/domain/model/entity/workflow-engine";
import { workflowArtifactDefinitionOptions } from "@/features/workflow-engine/workflow-artifact-definitions";

const DEMO_STEP_DEFINITION_CREATED_AT = "2026-05-20T00:00:00.000Z";
const DEFAULT_STEP_MODEL: SupportedStepModel = "gpt-5.4";
const DEFAULT_REASONING_EFFORT = "medium";

function resolveProviderKeyFromModel(model: string) {
  if (model.startsWith("gpt-")) {
    return "codex";
  }
  if (model.startsWith("gemini-")) {
    return "gemini";
  }
  if (model.startsWith("claude-")) {
    return "claude";
  }

  return "codex";
}

function normalizeModel(model: string | null | undefined, fallback: SupportedStepModel = DEFAULT_STEP_MODEL) {
  return (model?.trim() as SupportedStepModel) || fallback;
}

function normalizeReasoningEffort(
  reasoningEffort: string | null | undefined,
  fallback: string = DEFAULT_REASONING_EFFORT,
) {
  return reasoningEffort?.trim() || fallback;
}

const BUILT_IN_STEP_ARTIFACT_BINDINGS: Record<
  StepType,
  {
    inputArtifactDefinitions?: string[];
    outputArtifactDefinitions?: string[];
  }
> = {
  business_idea: {
    outputArtifactDefinitions: ["business_idea_artifact"],
  },
  feature_intake: {
    outputArtifactDefinitions: ["feature_intake_artifact"],
  },
  business_summary: {
    inputArtifactDefinitions: ["business_idea_artifact", "feature_intake_artifact"],
    outputArtifactDefinitions: ["business_summary_artifact"],
  },
  product_spec: {
    inputArtifactDefinitions: ["business_summary_artifact"],
    outputArtifactDefinitions: ["product_spec_artifact"],
  },
  tech_spec: {
    outputArtifactDefinitions: ["tech_spec_artifact"],
  },
  make_plan_coding: {
    inputArtifactDefinitions: ["tech_spec_artifact"],
    outputArtifactDefinitions: ["coding_plan_artifact"],
  },
  create_architecture: {
    inputArtifactDefinitions: ["business_summary_artifact"],
    outputArtifactDefinitions: ["architecture_artifact"],
  },
  tdd: {
    inputArtifactDefinitions: [
      "tech_spec_artifact",
      "coding_plan_artifact",
      "architecture_artifact",
    ],
    outputArtifactDefinitions: ["tdd_plan_artifact"],
  },
  task_breakdown: {
    outputArtifactDefinitions: ["task_breakdown_artifact"],
  },
  code_review_loop: {
    inputArtifactDefinitions: ["coding_plan_artifact", "tdd_plan_artifact"],
    outputArtifactDefinitions: [
      "coding_implementation_checklist_artifact",
      "code_review_summary_artifact",
    ],
  },
  release_readiness: {
    inputArtifactDefinitions: ["code_review_summary_artifact"],
    outputArtifactDefinitions: ["release_readiness_artifact"],
  },
  issue_analysis: {
    outputArtifactDefinitions: ["root_cause_analysis_artifact"],
  },
  analytics_review: {
    outputArtifactDefinitions: ["usage_analytics_artifact"],
  },
  project_analysis: {
    outputArtifactDefinitions: ["project_analysis_artifact"],
  },
  telegram_notification: {},
  code_traceability: {
    outputArtifactDefinitions: ["code_traceability_artifact"],
  },
  onboarding_walkthrough: {},
};

export class InMemoryWorkflowEngineGateway implements WorkflowEngineGateway {
  private artifactDefinitions: ArtifactDefinition[] = workflowArtifactDefinitionOptions.map(
    (definition) => ({
      key: definition.key,
      name: definition.label,
      description: `${definition.label} workflow artifact`,
      localPathTemplate: `.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/${definition.defaultFileName}`,
      remotePathTemplate: `artifacts/{projectId}/{workflowRunId}/{stepType}/${definition.defaultFileName}`,
      defaultFileName: definition.defaultFileName,
      createdAt: DEMO_STEP_DEFINITION_CREATED_AT,
      updatedAt: DEMO_STEP_DEFINITION_CREATED_AT,
    })
  );

  private stepDefinitions: StepDefinition[] = [
    {
      stepType: "business_idea",
      name: "Business Idea",
      description: "Capture and refine a raw business idea",
      requiredMcps: [],
      requiredSkills: ["business_analyst_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "feature_intake",
      name: "Feature Intake",
      description: "Read and structure feature requirements from Jira",
      requiredMcps: ["jira"],
      requiredSkills: ["feature_intake_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "business_summary",
      name: "Business Summary",
      description: "Summarize business requirements into a PRD",
      requiredMcps: [],
      requiredSkills: ["business_summary_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "product_spec",
      name: "Product Spec",
      description: "Generate a detailed product specification",
      requiredMcps: [],
      requiredSkills: ["product_spec_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "tech_spec",
      name: "Tech Spec",
      description: "Generate a technical specification",
      requiredMcps: [],
      requiredSkills: ["tech_spec_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "make_plan_coding",
      name: "Make Plan Coding",
      description: "Create a step-by-step coding plan",
      requiredMcps: [],
      requiredSkills: ["make_plan_coding_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "create_architecture",
      name: "Create Architecture",
      description: "Design system architecture and components",
      requiredMcps: [],
      requiredSkills: ["create_architecture_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "tdd",
      name: "TDD",
      description: "Create unit test signatures matching business requirements",
      requiredMcps: [],
      requiredSkills: ["tdd_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "task_breakdown",
      name: "Task Breakdown",
      description: "Break coding plan into developer tasks with master schedule",
      requiredMcps: [],
      requiredSkills: ["task_breakdown_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "code_review_loop",
      name: "Code/Review Loop",
      description: "Autonomously code, test, and review until passing",
      requiredMcps: [],
      requiredSkills: ["coding_skill", "review_skill"],
      agentType: "autonomous",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "release_readiness",
      name: "Release Readiness",
      description: "Verify release criteria are met",
      requiredMcps: [],
      requiredSkills: ["release_readiness_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "issue_analysis",
      name: "Issue Analysis",
      description: "Analyze crash reports and issues for root cause",
      requiredMcps: ["firebase", "jira"],
      requiredSkills: ["issue_analysis_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "analytics_review",
      name: "Analytics Review",
      description: "Review usage data and user behavior insights",
      requiredMcps: ["firebase"],
      requiredSkills: ["analytics_review_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "project_analysis",
      name: "Project Analysis",
      description: "Analyze project health and process bottlenecks",
      requiredMcps: ["google_drive"],
      requiredSkills: ["project_analysis_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "telegram_notification",
      name: "Telegram Notification",
      description: "Send workflow status to Telegram chat",
      requiredMcps: ["telegram"],
      requiredSkills: ["notification_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "code_traceability",
      name: "Code Traceability",
      description: "Trace code/bug back to original Jira ticket",
      requiredMcps: ["jira"],
      requiredSkills: ["code_traceability_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
    {
      stepType: "onboarding_walkthrough",
      name: "Onboarding Walkthrough",
      description: "Generate product/codebase summary for new members",
      requiredMcps: ["google_drive"],
      requiredSkills: ["onboarding_skill"],
      agentType: "standard",
      reasoningEffort: DEFAULT_REASONING_EFFORT,
    },
  ].map((step) => ({
    ...step,
    promptBase: deriveStepPromptBase(step),
    model: DEFAULT_STEP_MODEL,
    ...BUILT_IN_STEP_ARTIFACT_BINDINGS[step.stepType],
    createdAt: DEMO_STEP_DEFINITION_CREATED_AT,
    updatedAt: DEMO_STEP_DEFINITION_CREATED_AT,
  }) as StepDefinition);

  private workflows: Workflow[] = [];
  private workflowSteps: WorkflowStep[] = [];
  private workflowRuns: WorkflowRun[] = [];
  private workflowRunSteps: WorkflowRunStep[] = [];
  private workflowRunLogs: WorkflowRunLog[] = [];
  private artifactRuns: ArtifactRun[] = [];

  constructor() {
    this.seedTemplates();
  }

  private seedTemplates() {
    const templates = [
      {
        name: "Bug Fix Flow",
        description: "Traceability -> Issue Analysis -> Tech Spec -> Plan -> Code/Review -> Release",
        steps: ["code_traceability", "issue_analysis", "tech_spec", "make_plan_coding", "code_review_loop", "release_readiness"],
      },
      {
        name: "Pre-defined Feature",
        description: "Tech Spec -> Plan -> Architecture -> TDD -> Code/Review -> Release -> Notify",
        steps: ["tech_spec", "make_plan_coding", "create_architecture", "tdd", "code_review_loop", "release_readiness", "telegram_notification"],
      },
      {
        name: "Bug Traceability",
        description: "Code Traceability single step",
        steps: ["code_traceability"],
      },
      {
        name: "Onboarding",
        description: "Onboarding Walkthrough single step",
        steps: ["onboarding_walkthrough"],
      },
      {
        name: "Full End-to-End",
        description: "Complete Solo Dev pipeline from Business Idea to Notification",
        steps: ["business_idea", "feature_intake", "business_summary", "product_spec", "tech_spec", "make_plan_coding", "create_architecture", "tdd", "code_review_loop", "release_readiness", "telegram_notification"],
      },
      {
        name: "Fast-Track Business",
        description: "Product Spec -> Tech Spec -> Plan -> Arch -> TDD -> Code/Review -> Release",
        steps: ["product_spec", "tech_spec", "make_plan_coding", "create_architecture", "tdd", "code_review_loop", "release_readiness"],
      },
      {
        name: "Task Breakdown",
        description: "Tech Spec -> Plan -> Task Breakdown",
        steps: ["tech_spec", "make_plan_coding", "task_breakdown"],
      },
      {
        name: "Root Cause Analysis",
        description: "Traceability -> Issue Analysis -> Task Breakdown -> Notify",
        steps: ["code_traceability", "issue_analysis", "task_breakdown", "telegram_notification"],
      },
      {
        name: "Analytics & Usage",
        description: "Analytics Review single step",
        steps: ["analytics_review"],
      },
      {
        name: "Product Process & Analysis",
        description: "Business Idea -> Feature Intake -> Business Summary -> Product Spec -> Project Analysis -> Analytics Review",
        steps: ["business_idea", "feature_intake", "business_summary", "product_spec", "project_analysis", "analytics_review"],
      },
    ];

    templates.forEach((t, index) => {
      const wId = `demo-wf-${index}`;
      this.workflows.push({
        id: wId,
        projectId: null,
        name: t.name,
        description: t.description,
        isTemplate: true,
        providerOverride: null,
        modelOverride: null,
        createdBy: "seed",
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      });

      t.steps.forEach((stepType, idx) => {
        this.workflowSteps.push({
          id: `demo-wfs-${wId}-${idx}`,
          workflowId: wId,
          stepType: stepType as StepType,
          orderIndex: idx,
          isEnabled: true,
          providerOverride: null,
          modelOverride: null,
          requiresApproval: true,
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        });
      });
    });
  }

  async listArtifactDefinitions(): Promise<ArtifactDefinition[]> {
    return this.artifactDefinitions;
  }

  async saveArtifactDefinition(definition: ArtifactDefinition): Promise<ArtifactDefinition> {
    const next = { ...definition };
    const index = this.artifactDefinitions.findIndex((item) => item.key === next.key);
    if (index >= 0) {
      this.artifactDefinitions[index] = next;
    } else {
      this.artifactDefinitions.unshift(next);
    }
    return next;
  }

  async listArtifactRuns(projectId?: string): Promise<ArtifactRun[]> {
    if (!projectId) {
      return this.artifactRuns;
    }
    return this.artifactRuns.filter((artifact) => artifact.projectId === projectId);
  }

  async listStepDefinitions(): Promise<StepDefinition[]> {
    return this.stepDefinitions;
  }

  async saveStepDefinition(step: StepDefinition): Promise<StepDefinition> {
    const next = {
      ...step,
      promptBase: step.promptBase ?? deriveStepPromptBase(step),
      teamRole: step.teamRole ?? null,
      subagent: step.subagent ?? null,
      model: step.model,
    };
    const existingIndex = this.stepDefinitions.findIndex(
      (current) => current.stepType === next.stepType
    );

    if (existingIndex >= 0) {
      this.stepDefinitions[existingIndex] = next;
    } else {
      this.stepDefinitions.unshift(next);
    }

    return next;
  }

  async listWorkflows(projectId?: string): Promise<Workflow[]> {
    const visibleWorkflows = this.workflows.filter(
      (workflow) => workflow.createdBy !== "flowpilot-runtime"
    );
    if (!projectId) return visibleWorkflows;

    return visibleWorkflows.filter(
      (workflow) => workflow.projectId === null || workflow.projectId === projectId
    );
  }

  async getWorkflowDetail(workflowId: string): Promise<Workflow | null> {
    const w = this.workflows.find((item) => item.id === workflowId);
    if (!w) return null;
    return {
      ...w,
      steps: this.workflowSteps.filter((s) => s.workflowId === workflowId),
    };
  }

  async saveWorkflow(
    workflow: Omit<Partial<Workflow>, "steps"> & { steps: Partial<WorkflowStep>[] }
  ): Promise<Workflow> {
    const isNew = !workflow.id;
    const wId = workflow.id || `wf-${Math.random().toString(36).slice(2, 9)}`;
    const resolvedWorkflowModel = normalizeModel(workflow.modelOverride);
    const resolvedWorkflowReasoning = normalizeReasoningEffort(workflow.reasoningEffortOverride);
    const resolvedWorkflowProvider = resolveProviderKeyFromModel(resolvedWorkflowModel);

    if (isNew) {
      const newW: Workflow = {
        id: wId,
        projectId: workflow.projectId || null,
        name: workflow.name || "Untitled",
        description: workflow.description || "",
        isTemplate: workflow.isTemplate ?? false,
        providerOverride: resolvedWorkflowProvider,
        modelOverride: resolvedWorkflowModel,
        reasoningEffortOverride: resolvedWorkflowReasoning,
        createdBy: "demo-user",
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      };
      this.workflows.unshift(newW);
    } else {
      const match = this.workflows.find((item) => item.id === wId);
      if (match) {
        match.projectId = workflow.projectId ?? match.projectId;
        match.name = workflow.name ?? match.name;
        match.description = workflow.description ?? match.description;
        match.isTemplate = workflow.isTemplate ?? match.isTemplate;
        match.providerOverride = resolvedWorkflowProvider;
        match.modelOverride = resolvedWorkflowModel;
        match.reasoningEffortOverride = resolvedWorkflowReasoning;
        match.updatedAt = new Date().toISOString();
      }
    }

    // Replace steps
    this.workflowSteps = this.workflowSteps.filter((s) => s.workflowId !== wId);
    if (workflow.steps) {
      workflow.steps.forEach((step, idx) => {
        this.workflowSteps.push({
          id: step.id || `wfs-${wId}-${idx}`,
          workflowId: wId,
          stepType: step.stepType!,
          orderIndex: step.orderIndex ?? idx,
          isEnabled: step.isEnabled ?? true,
          providerOverride: resolveProviderKeyFromModel(
            normalizeModel(step.modelOverride, resolvedWorkflowModel),
          ),
          modelOverride: normalizeModel(step.modelOverride, resolvedWorkflowModel),
          reasoningEffortOverride: normalizeReasoningEffort(
            step.reasoningEffortOverride,
            resolvedWorkflowReasoning,
          ),
          requiresApproval: step.requiresApproval ?? true,
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        });
      });
    }

    return (await this.getWorkflowDetail(wId))!;
  }

  async listWorkflowRuns(projectId?: string): Promise<WorkflowRun[]> {
    if (!projectId) return this.workflowRuns;
    return this.workflowRuns.filter((r) => r.projectId === projectId);
  }

  async getWorkflowRunDetail(
    runId: string
  ): Promise<{
    run: WorkflowRun;
    steps: WorkflowRunStep[];
    logs: WorkflowRunLog[];
  } | null> {
    const run = this.workflowRuns.find((r) => r.id === runId);
    if (!run) return null;

    const steps = this.workflowRunSteps.filter((s) => s.workflowRunId === runId);
    const logs = this.workflowRunLogs.filter((l) =>
      steps.some((s) => s.id === l.workflowRunStepId)
    );

    return { run, steps, logs };
  }

  async deleteWorkflowRuns(runIds: string[]): Promise<void> {
    const ids = new Set(runIds.filter((runId) => runId.trim().length > 0));
    if (ids.size === 0) {
      return;
    }

    const stepIdsToDelete = new Set<string>();
    for (let index = this.workflowRunSteps.length - 1; index >= 0; index -= 1) {
      const step = this.workflowRunSteps[index];
      if (ids.has(step.workflowRunId)) {
        stepIdsToDelete.add(step.id);
        this.workflowRunSteps.splice(index, 1);
      }
    }

    for (let index = this.workflowRuns.length - 1; index >= 0; index -= 1) {
      if (ids.has(this.workflowRuns[index].id)) {
        this.workflowRuns.splice(index, 1);
      }
    }

    for (let index = this.workflowRunLogs.length - 1; index >= 0; index -= 1) {
      if (stepIdsToDelete.has(this.workflowRunLogs[index].workflowRunStepId)) {
        this.workflowRunLogs.splice(index, 1);
      }
    }

    for (let index = this.artifactRuns.length - 1; index >= 0; index -= 1) {
      if (ids.has(this.artifactRuns[index].workflowRunId)) {
        this.artifactRuns.splice(index, 1);
      }
    }
  }

  async startWorkflowRun(request: WorkflowRunStartRequest): Promise<WorkflowRun> {
    let workflowId = request.workflowId ?? null;
    let workflowDetail =
      request.startMode === "workflow-definition" && workflowId
        ? await this.getWorkflowDetail(workflowId)
        : null;

    if (request.startMode === "single-step") {
      const selectedStep = this.stepDefinitions.find((step) => step.stepType === request.stepType);
      if (!selectedStep) {
        throw new Error("Workflow not found.");
      }
      const selectedModel = normalizeModel(selectedStep.model);
      const selectedReasoningEffort = normalizeReasoningEffort(selectedStep.reasoningEffort);

      workflowId = `runtime-${request.stepType}-${Math.random().toString(36).slice(2, 9)}`;
      const workflow: Workflow = {
        id: workflowId,
        projectId: request.projectId,
        name: `Single Step: ${selectedStep.name}`,
        description: `Runtime-generated single-step workflow for ${request.stepType}.`,
        isTemplate: false,
        providerOverride: resolveProviderKeyFromModel(selectedModel),
        modelOverride: selectedModel,
        reasoningEffortOverride: selectedReasoningEffort,
        createdBy: "flowpilot-runtime",
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      };
      const workflowStep: WorkflowStep = {
        id: `wfs-${workflowId}-0`,
        workflowId,
        stepType: selectedStep.stepType,
        orderIndex: 0,
        isEnabled: true,
        providerOverride: resolveProviderKeyFromModel(selectedModel),
        modelOverride: selectedModel,
        reasoningEffortOverride: selectedReasoningEffort,
        requiresApproval: false,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      };

      this.workflows.unshift(workflow);
      this.workflowSteps.push(workflowStep);
      workflowDetail = {
        ...workflow,
        steps: [workflowStep],
      };
    }

    if (!workflowDetail || !workflowId) throw new Error("Workflow not found.");

    const runId = `run-${Math.random().toString(36).slice(2, 9)}`;
    const newRun: WorkflowRun = {
      id: runId,
      workflowId,
      projectId: request.projectId,
      status: "PENDING",
      provider: resolveProviderKeyFromModel(
        normalizeModel(workflowDetail.modelOverride),
      ),
      model: normalizeModel(workflowDetail.modelOverride),
      reasoningEffort: normalizeReasoningEffort(workflowDetail.reasoningEffortOverride),
      yoloMode: false,
      startedBy: "demo-user",
      startedAt: new Date().toISOString(),
      finishedAt: null,
      errorMessage: null,
    };
    this.workflowRuns.unshift(newRun);

    if (workflowDetail.steps) {
      workflowDetail.steps.forEach((step, idx) => {
        const wrsId = `wrs-${runId}-${idx}`;
        this.workflowRunSteps.push({
          id: wrsId,
          workflowRunId: runId,
          workflowStepId: step.id,
          executionOrderIndex: step.orderIndex,
          stepType: step.stepType,
          status: step.isEnabled ? (idx === 0 ? "RUNNING" : "PENDING") : "SKIPPED",
          artifactId: null,
          promptCacheId: null,
          rejectionNote: null,
          retryCount: 0,
          startedAt: idx === 0 ? new Date().toISOString() : null,
          finishedAt: null,
          errorMessage: null,
        });

        if (step.isEnabled) {
          this.workflowRunLogs.push({
            id: `log-${wrsId}-1`,
            workflowRunStepId: wrsId,
            logLevel: "info",
            message: `Materialized step ${step.stepType} in execution pipeline.`,
            createdAt: new Date().toISOString(),
          });
          this.workflowRunLogs.push({
            id: `log-${wrsId}-begin`,
            workflowRunStepId: wrsId,
            logLevel: "info",
            message: `Begin prompt: ${request.beginPrompt}`,
            createdAt: new Date().toISOString(),
          });
        }
      });
    }

    // Auto-advance some steps for a nice interactive demo UX
    this.simulateExecution(runId);

    return newRun;
  }

  private simulateExecution(runId: string) {
    setTimeout(() => {
      const steps = this.workflowRunSteps.filter((s) => s.workflowRunId === runId);
      const activeStep = steps.find((s) => s.status === "RUNNING");
      if (activeStep) {
        // Switch first step to WAITING_USER_APPROVAL to show interactive approval UI
        activeStep.status = "WAITING_USER_APPROVAL";
        activeStep.artifactId = `art-${activeStep.id}`;
        this.workflowRunLogs.push({
          id: `log-${activeStep.id}-2`,
          workflowRunStepId: activeStep.id,
          logLevel: "info",
          message: `Generation complete. Paused on approval gate.`,
          createdAt: new Date().toISOString(),
        });
      }
    }, 1500);
  }

  async toggleYoloMode(runId: string, yoloMode: boolean): Promise<WorkflowRun> {
    const run = this.workflowRuns.find((r) => r.id === runId);
    if (!run) throw new Error("Run not found.");
    run.yoloMode = yoloMode;

    if (yoloMode) {
      // If we enable YOLO mode, auto-advance any waiting approvals immediately!
      const steps = this.workflowRunSteps.filter((s) => s.workflowRunId === runId);
      const waiting = steps.find((s) => s.status === "WAITING_USER_APPROVAL");
      if (waiting) {
        this.submitStepApproval(waiting.id, true);
      }
    }

    return run;
  }

  async submitStepApproval(
    stepId: string,
    approve: boolean,
    comment?: string
  ): Promise<WorkflowRunStep> {
    const step = this.workflowRunSteps.find((s) => s.id === stepId);
    if (!step) throw new Error("Step not found.");

    if (approve) {
      step.status = "DONE";
      step.finishedAt = new Date().toISOString();
      this.workflowRunLogs.push({
        id: `log-${step.id}-appr`,
        workflowRunStepId: step.id,
        logLevel: "info",
        message: `Step approved and completed.`,
        createdAt: new Date().toISOString(),
      });

      // Advance to next step
      const steps = this.workflowRunSteps.filter((s) => s.workflowRunId === step.workflowRunId);
      const myIdx = steps.findIndex((s) => s.id === stepId);
      const nextStep = steps[myIdx + 1];

      if (nextStep) {
        nextStep.status = "RUNNING";
        nextStep.startedAt = new Date().toISOString();
        this.workflowRunLogs.push({
          id: `log-${nextStep.id}-start`,
          workflowRunStepId: nextStep.id,
          logLevel: "info",
          message: `Initiating runner for step: ${nextStep.stepType}`,
          createdAt: new Date().toISOString(),
        });

        // Simulate next step complete or approval gate
        setTimeout(() => {
          if (nextStep.stepType === "telegram_notification") {
            nextStep.status = "DONE";
            nextStep.finishedAt = new Date().toISOString();
          } else {
            nextStep.status = "WAITING_USER_APPROVAL";
            nextStep.artifactId = `art-${nextStep.id}`;
          }
        }, 2000);
      } else {
        // Complete run
        const run = this.workflowRuns.find((r) => r.id === step.workflowRunId);
        if (run) {
          run.status = "DONE";
          run.finishedAt = new Date().toISOString();
        }
      }
    } else {
      step.status = "PENDING";
      step.rejectionNote = comment || "Rejected by reviewer";
      step.retryCount += 1;
      this.workflowRunLogs.push({
        id: `log-${step.id}-rej`,
        workflowRunStepId: step.id,
        logLevel: "warn",
        message: `Rejection received: "${step.rejectionNote}". Resetting to PENDING for regeneration (Retry #${step.retryCount})`,
        createdAt: new Date().toISOString(),
      });

      // Simulate re-execution
      setTimeout(() => {
        step.status = "RUNNING";
        this.workflowRunLogs.push({
          id: `log-${step.id}-retry-run`,
          workflowRunStepId: step.id,
          logLevel: "info",
          message: `Regenerating output with feedback incorporated: "${step.rejectionNote}"`,
          createdAt: new Date().toISOString(),
        });

        setTimeout(() => {
          step.status = "WAITING_USER_APPROVAL";
          this.workflowRunLogs.push({
            id: `log-${step.id}-retry-gate`,
            workflowRunStepId: step.id,
            logLevel: "info",
            message: `Revised artifact output ready. Paused on approval gate.`,
            createdAt: new Date().toISOString(),
          });
        }, 2000);
      }, 1000);
    }

    return step;
  }
}
