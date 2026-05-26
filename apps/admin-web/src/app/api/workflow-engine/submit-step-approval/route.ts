import { handleWorkflowSubmitStepApproval } from "@/features/workflow-engine/submit-step-approval-http-handler";

export async function POST(request: Request) {
  return handleWorkflowSubmitStepApproval(request);
}
