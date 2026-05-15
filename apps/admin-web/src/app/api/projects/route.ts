import { redirect } from "next/navigation";
import { z } from "zod";

import { createGatewayBundle } from "@/data/repository/factory";
import { CreateProjectUseCase } from "@/domain/usecase/projects/create-project-usecase";

const createProjectSchema = z.object({
  name: z.string().min(3),
  description: z.string().min(10),
  platform: z.enum(["android", "ios", "web", "multi"]),
  repositoryUrl: z.string().url(),
});

export async function POST(request: Request) {
  const formData = await request.formData();
  const payload = createProjectSchema.parse({
    name: formData.get("name"),
    description: formData.get("description"),
    platform: formData.get("platform"),
    repositoryUrl: formData.get("repositoryUrl"),
  });

  const gateways = await createGatewayBundle();
  await new CreateProjectUseCase(gateways.projectGateway).execute(payload);

  redirect("/projects");
}
