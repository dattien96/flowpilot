import { redirect } from "next/navigation";
import { z } from "zod";

import { createGatewayBundle } from "@/data/repository/factory";
import { CreateFeatureUseCase } from "@/domain/usecase/features/create-feature-usecase";

const createFeatureSchema = z.object({
  projectId: z.string().min(1),
  title: z.string().min(3),
  businessGoal: z.string().min(10),
  userProblem: z.string().min(10),
  expectedFlow: z.string().min(10),
  acceptanceCriteria: z.string().min(10),
  priority: z.enum(["low", "medium", "high"]),
});

export async function POST(request: Request) {
  const formData = await request.formData();
  const payload = createFeatureSchema.parse({
    projectId: formData.get("projectId"),
    title: formData.get("title"),
    businessGoal: formData.get("businessGoal"),
    userProblem: formData.get("userProblem"),
    expectedFlow: formData.get("expectedFlow"),
    acceptanceCriteria: formData.get("acceptanceCriteria"),
    priority: formData.get("priority"),
  });

  const gateways = await createGatewayBundle();
  await new CreateFeatureUseCase(gateways.featureGateway).execute(payload);

  redirect("/features");
}
