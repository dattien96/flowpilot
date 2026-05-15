import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type { LocalRunnerStorageDriverRequest } from "@/domain/model/entity/local-runner";

export class UpdateStorageDriverUseCase {
  constructor(private readonly localRunnerGateway: LocalRunnerGateway) {}

  execute(request: LocalRunnerStorageDriverRequest) {
    return this.localRunnerGateway.saveStorageDriver(request);
  }
}
