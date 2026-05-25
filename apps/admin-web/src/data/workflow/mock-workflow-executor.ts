import type { OutputType } from "@/domain/constant/status";
import type { WorkflowExecutorGateway, WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type { Project } from "@/domain/model/entity/project";
import { saveWorkflowArtifact } from "@/data/repository/artifacts/local-artifact-store";

function buildOutputMarkdown(
  outputType: OutputType,
  project: Project,
) {
  const titleMap: Record<OutputType, string> = {
    business_summary: "Business Summary",
    product_spec: "Product Spec",
    android_tech_spec: "Android Tech Spec",
    task_breakdown: "Task Breakdown",
    test_plan: "Test Plan",
    risk_report: "Risk Report",
  };

  return `# ${titleMap[outputType]}

## Project
${project.name}

## Working Notes
- Continue the workflow using project context, prior outputs, and any selected runtime context.
`;
}

export class MockWorkflowExecutor implements WorkflowExecutorGateway {
  constructor(
    private readonly workflowGateway: WorkflowGateway,
    private readonly projectLookup: (projectId: string) => Promise<Project | null>,
  ) {}

  async executeUntilPause(runId: string) {
    const detail = await this.workflowGateway.getWorkflowRunDetail(runId);

    if (!detail) {
      throw new Error("Workflow run not found.");
    }

    const project = await this.projectLookup(detail.run.projectId);
    if (!project) {
      throw new Error("Project missing for workflow execution.");
    }

    for (const step of detail.steps) {
      if (step.status !== "pending") {
        continue;
      }

      await this.workflowGateway.updateWorkflowStep(step.id, {
        status: step.stepType === "approval" ? "waiting_approval" : "running",
        startedAt: new Date().toISOString(),
      });

      if (step.stepType === "tool") {
        await this.workflowGateway.updateWorkflowStep(step.id, {
          status: "completed",
          completedAt: new Date().toISOString(),
        });
        continue;
      }

      if (step.stepType === "approval") {
        await this.workflowGateway.updateWorkflowRun(runId, {
          status: "waiting_approval",
          currentStepKey: step.stepKey,
        });

        return;
      }

      if (step.outputId) {
        continue;
      }

      const outputType = detail.run.workflowDefinitionId && detail.steps
        ? detail.steps.find((item) => item.id === step.id)?.stepKey.includes("business")
          ? "business_summary"
          : undefined
        : undefined;

      const normalizedOutputType = step.stepKey.replace("generate_", "") as OutputType;

      const outputId = `output_${step.id}`;

      await this.workflowGateway.createOutput({
        id: outputId,
        workflowRunId: runId,
        workflowStepId: step.id,
        projectId: detail.run.projectId,
        outputType: outputType ?? normalizedOutputType,
        version: 1,
        title: step.stepName,
        contentMarkdown: buildOutputMarkdown(
          outputType ?? normalizedOutputType,
          project,
        ),
        isApproved: false,
        createdAt: new Date().toISOString(),
      });

      await saveWorkflowArtifact({
        artifactId: outputId,
        title: step.stepName,
        projectId: detail.run.projectId,
        workflowRunId: runId,
        workflowStepKey: step.stepKey,
        providerKey: "mock",
        contentMarkdown: buildOutputMarkdown(
          outputType ?? normalizedOutputType,
          project,
        ),
        promptText: `Generate ${step.stepName} for ${project.name}`,
        stdoutText: "",
        stderrText: "",
        commandText: "mock-executor",
        sourceKind: "workflow_output",
      });

      await this.workflowGateway.createLog({
        id: `log_${step.id}`,
        workflowRunId: runId,
        workflowStepId: step.id,
        provider: "mock",
        model: "mock-planner-v1",
        inputTokens: 240 + step.sequenceIndex * 17,
        outputTokens: 420 + step.sequenceIndex * 21,
        costEstimate: Number((0.002 + step.sequenceIndex * 0.0004).toFixed(4)),
        latencyMs: 180 + step.sequenceIndex * 45,
        status: "success",
        createdAt: new Date().toISOString(),
      });

      await this.workflowGateway.updateWorkflowStep(step.id, {
        status: "completed",
        outputId,
        completedAt: new Date().toISOString(),
      });
    }

    await this.workflowGateway.updateWorkflowRun(runId, {
      status: "completed",
      currentStepKey: null,
      completedAt: new Date().toISOString(),
    });
  }
}
