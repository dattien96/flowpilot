import { NextResponse } from "next/server";
import { z } from "zod";

import { createGatewayBundle } from "@/data/repository/factory";
import { GetStorageDriverUseCase } from "@/domain/usecase/artifacts/get-storage-driver-usecase";
import { UpdateStorageDriverUseCase } from "@/domain/usecase/artifacts/update-storage-driver-usecase";
import { ValidateStorageDriverUseCase } from "@/domain/usecase/artifacts/validate-storage-driver-usecase";

const storageDriverSchema = z.object({
  driverKey: z.string().min(1),
  enabled: z.boolean(),
  remoteRootPath: z.string().default(""),
  remoteFolderName: z.string().default("FlowPilot"),
});

export async function GET() {
  const gateways = await createGatewayBundle();
  const result = await new GetStorageDriverUseCase(gateways.localRunnerGateway).execute();
  return NextResponse.json(result);
}

export async function PUT(request: Request) {
  try {
    const payload = storageDriverSchema.parse(await request.json());
    const gateways = await createGatewayBundle();
    const result = await new UpdateStorageDriverUseCase(gateways.localRunnerGateway).execute(payload);
    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to save storage driver.",
      },
      { status: 400 },
    );
  }
}

export async function POST() {
  try {
    const gateways = await createGatewayBundle();
    const result = await new ValidateStorageDriverUseCase(gateways.localRunnerGateway).execute();
    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to validate storage driver.",
      },
      { status: 400 },
    );
  }
}
