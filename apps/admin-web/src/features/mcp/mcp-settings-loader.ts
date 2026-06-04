import { createGatewayBundle } from "@/data/repository/browser-factory";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { ListLocalMcpBackendsUseCase } from "@/domain/usecase/local-runner/list-local-mcp-backends-usecase";
import { ListProjectsUseCase } from "@/domain/usecase/projects/list-projects-usecase";

export async function loadMcpSettingsData() {
  const gateways = await createGatewayBundle();
  const [health, backends, projects, allIntegrations] = await Promise.all([
    new CheckLocalRunnerHealthUseCase(gateways.localRunnerGateway).execute(),
    new ListLocalMcpBackendsUseCase(gateways.localRunnerGateway).execute(),
    new ListProjectsUseCase(gateways.projectGateway).execute(),
    gateways.integrationGateway.listAllIntegrations(),
  ]);

  return { allIntegrations, backends, health, projects };
}

export type McpSettingsLoaderData = Awaited<ReturnType<typeof loadMcpSettingsData>>;
