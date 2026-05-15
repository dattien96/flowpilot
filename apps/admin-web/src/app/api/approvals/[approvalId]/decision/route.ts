import { redirect } from "next/navigation";
import { NextResponse } from "next/server";
import { z } from "zod";

import { createGatewayBundle } from "@/data/repository/factory";
import { SubmitApprovalDecisionUseCase } from "@/domain/usecase/approvals/submit-approval-decision-usecase";

const approvalDecisionSchema = z.object({
  decision: z.enum(["approved", "rejected", "changes_requested"]),
  comment: z.string().optional(),
});

export async function POST(
  request: Request,
  context: { params: Promise<{ approvalId: string }> },
) {
  const { approvalId } = await context.params;
  const contentType = request.headers.get("content-type") ?? "";
  let parsedInput: z.infer<typeof approvalDecisionSchema>;

  if (contentType.includes("application/json")) {
    parsedInput = approvalDecisionSchema.parse(await request.json());
  } else {
    const formData = await request.formData();
    parsedInput = approvalDecisionSchema.parse({
      decision: formData.get("decision"),
      comment: formData.get("comment"),
    });
  }

  const gateways = await createGatewayBundle();
  const result = await new SubmitApprovalDecisionUseCase(
    gateways.workflowGateway,
    gateways.workflowExecutor,
  ).execute({
    approvalId,
    decision: parsedInput.decision,
    comment: parsedInput.comment,
  });

  if (contentType.includes("application/json")) {
    return NextResponse.json(result);
  }

  redirect(`/workflow-runs/${result?.run.id ?? ""}`);
}
