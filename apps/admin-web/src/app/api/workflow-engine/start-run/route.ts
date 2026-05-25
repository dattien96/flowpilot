import { handleWorkflowStartRun } from "@/features/workflow-engine/start-run-http-handler";

export async function POST(request: Request) {
  return handleWorkflowStartRun(request);
}
