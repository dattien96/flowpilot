import { handleWorkflowSubmitGoogleDriveWriteApproval } from "@/features/workflow-engine/submit-google-drive-write-approval-http-handler";

export async function POST(request: Request) {
  return handleWorkflowSubmitGoogleDriveWriteApproval(request);
}
