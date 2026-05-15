import { createGatewayBundle } from "@/data/repository/factory";
import { GetStorageDriverUseCase } from "@/domain/usecase/artifacts/get-storage-driver-usecase";
import { ListArtifactsUseCase } from "@/domain/usecase/artifacts/list-artifacts-usecase";
import { ArtifactBrowserPanel } from "@/presentation/components/artifacts/artifact-browser-panel";

export default async function OutputsPage() {
  const gateways = await createGatewayBundle();
  const [artifacts, storageDriver] = await Promise.all([
    new ListArtifactsUseCase(gateways.workflowGateway, gateways.localRunnerGateway).execute(),
    new GetStorageDriverUseCase(gateways.localRunnerGateway).execute(),
  ]);

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Artifact Browser
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">
          Stored workflow and runner artifacts
        </h1>
      </header>
      <ArtifactBrowserPanel artifacts={artifacts} storageDriver={storageDriver} />
    </div>
  );
}
