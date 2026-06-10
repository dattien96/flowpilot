import type {
  ArtifactDefinition,
  ArtifactRun,
  StepDefinition,
  Workflow,
  WorkflowRunStartRequest,
  WorkflowRun,
  WorkflowRunLog,
  WorkflowRunStep,
  WorkflowStep,
  SupportedModel,
} from "@/domain/model/entity/workflow-engine";

export interface WorkflowEngineGateway {
  listSupportedModels(): Promise<SupportedModel[]>;
  createSupportedModel(
    model: Omit<SupportedModel, "id" | "createdAt" | "updatedAt">
  ): Promise<SupportedModel>;
  updateSupportedModel(
    id: string,
    model: Partial<Omit<SupportedModel, "id" | "createdAt" | "updatedAt">>
  ): Promise<SupportedModel>;
  deleteSupportedModel(id: string): Promise<void>;

  listArtifactDefinitions(): Promise<ArtifactDefinition[]>;
  saveArtifactDefinition(definition: ArtifactDefinition): Promise<ArtifactDefinition>;
  listArtifactRuns(projectId?: string): Promise<ArtifactRun[]>;
  listStepDefinitions(): Promise<StepDefinition[]>;
  saveStepDefinition(step: StepDefinition): Promise<StepDefinition>;
  listWorkflows(projectId?: string): Promise<Workflow[]>;
  getWorkflowDetail(workflowId: string): Promise<Workflow | null>;
  saveWorkflow(
    workflow: Omit<Partial<Workflow>, "steps"> & { steps: Partial<WorkflowStep>[] }
  ): Promise<Workflow>;
  listWorkflowRuns(projectId?: string): Promise<WorkflowRun[]>;
  getWorkflowRunDetail(
    runId: string
  ): Promise<{
    run: WorkflowRun;
    steps: WorkflowRunStep[];
    logs: WorkflowRunLog[];
  } | null>;
  deleteWorkflowRuns(runIds: string[]): Promise<void>;
  startWorkflowRun(request: WorkflowRunStartRequest): Promise<WorkflowRun>;
  toggleYoloMode(runId: string, yoloMode: boolean): Promise<WorkflowRun>;
  submitStepApproval(
    stepId: string,
    approve: boolean,
    comment?: string
  ): Promise<WorkflowRunStep>;
  submitGoogleDriveWriteApproval(
    stepId: string,
    decision: "approved" | "rejected",
    comment?: string,
  ): Promise<WorkflowRunStep>;
}
