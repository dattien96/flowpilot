import type {
  LocalRunnerFlow,
  LocalRunnerHealth,
  LocalRunnerProvider,
  LocalRunnerPromptExecutionRequest,
  LocalRunnerPromptExecutionResult,
  LocalRunnerSkill,
} from "@/domain/model/entity/local-runner";

export interface LocalRunnerGateway {
  getHealth(): Promise<LocalRunnerHealth>;
  listProviders(): Promise<LocalRunnerProvider[]>;
  listSkills(): Promise<LocalRunnerSkill[]>;
  listFlows(): Promise<LocalRunnerFlow[]>;
  executePrompt(
    request: LocalRunnerPromptExecutionRequest,
  ): Promise<LocalRunnerPromptExecutionResult>;
}
