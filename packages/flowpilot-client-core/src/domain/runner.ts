export interface RunnerHealth {
  cwd: string | null;
  os: string | null;
  startedAt: string | null;
  version: string | null;
}

export interface CompatVersionInfo {
  testedClaudeVersion: string;
  installedClaudeVersion: string;
  testedCodexVersion: string;
  installedCodexVersion: string;
}

export interface CompatConfig {
  testedClaudeVersion: string;
  testedCodexVersion: string;
}

export interface CompatItem {
  name: string;
  status: "pass" | "warn" | "fail";
  detail: string;
}

export interface CompatCheckResult extends CompatVersionInfo {
  items: CompatItem[];
  passed: number;
  warned: number;
  failed: number;
}

export interface RunnerRepository {
  loadHealth(): Promise<RunnerHealth>;
  loadCompatConfig(): Promise<CompatConfig>;
  saveCompatConfig(input: CompatConfig): Promise<CompatConfig>;
  loadCompatInfo(): Promise<CompatVersionInfo>;
  runCompatCheck(): Promise<CompatCheckResult>;
  runCompatDeepCheck(): Promise<CompatCheckResult>;
}

export class LoadRunnerHealthUseCase {
  constructor(private readonly runnerRepository: RunnerRepository) {}

  execute(): Promise<RunnerHealth> {
    return this.runnerRepository.loadHealth();
  }
}

export class LoadCompatInfoUseCase {
  constructor(private readonly runnerRepository: RunnerRepository) {}

  execute(): Promise<CompatVersionInfo> {
    return this.runnerRepository.loadCompatInfo();
  }
}

export class LoadCompatConfigUseCase {
  constructor(private readonly runnerRepository: RunnerRepository) {}

  execute(): Promise<CompatConfig> {
    return this.runnerRepository.loadCompatConfig();
  }
}

export class SaveCompatConfigUseCase {
  constructor(private readonly runnerRepository: RunnerRepository) {}

  execute(input: CompatConfig): Promise<CompatConfig> {
    return this.runnerRepository.saveCompatConfig(input);
  }
}

export class RunCompatCheckUseCase {
  constructor(private readonly runnerRepository: RunnerRepository) {}

  execute(): Promise<CompatCheckResult> {
    return this.runnerRepository.runCompatCheck();
  }
}

export class RunCompatDeepCheckUseCase {
  constructor(private readonly runnerRepository: RunnerRepository) {}

  execute(): Promise<CompatCheckResult> {
    return this.runnerRepository.runCompatDeepCheck();
  }
}
