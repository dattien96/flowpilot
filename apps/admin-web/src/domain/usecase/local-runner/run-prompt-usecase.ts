import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type { LocalRunnerPromptExecutionRequest } from "@/domain/model/entity/local-runner";

export class RunPromptUseCase {
  constructor(private readonly localRunnerGateway: LocalRunnerGateway) {}

  async execute(request: LocalRunnerPromptExecutionRequest) {
    const prompt = request.prompt.trim();

    if (!prompt) {
      throw new Error("Prompt is required.");
    }

    if (!request.providerKey.trim()) {
      throw new Error("Provider is required.");
    }

    return this.localRunnerGateway.executePrompt({
      ...request,
      prompt,
    });
  }
}
