export interface RunnerHealth {
  cwd: string | null;
  os: string | null;
  startedAt: string | null;
  version: string | null;
}

export interface RunnerRepository {
  loadHealth(): Promise<RunnerHealth>;
}

export class LoadRunnerHealthUseCase {
  constructor(private readonly runnerRepository: RunnerRepository) {}

  execute(): Promise<RunnerHealth> {
    return this.runnerRepository.loadHealth();
  }
}
