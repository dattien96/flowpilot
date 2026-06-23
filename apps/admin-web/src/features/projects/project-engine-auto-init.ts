import { createGatewayBundle } from "@/data/repository/browser-factory";

export async function autoInitProjectEngine(projectId: string, workingDirectory: string) {
  const trimmedWorkingDirectory = workingDirectory.trim();
  if (!trimmedWorkingDirectory) {
    return;
  }

  try {
    const gateways = await createGatewayBundle();
    await gateways.localRunnerGateway.initEngine(projectId, {
      workingDirectory: trimmedWorkingDirectory,
      trigger: "bind",
    });
  } catch (error) {
    console.warn(
      "FlowPilot engine auto-init failed",
      error instanceof Error ? error.message : error,
    );
  }
}
