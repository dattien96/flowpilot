export interface LocalRunnerHealth {
  status: "online" | "offline";
  runnerVersion: string | null;
  cwd: string | null;
  os: string | null;
  startedAt: string | null;
  baseUrl: string;
  errorMessage: string | null;
}

export interface LocalRunnerProvider {
  key: string;
  label: string;
  installed: boolean;
  version: string | null;
  binaryPath: string | null;
  authStatus: "authenticated" | "unauthenticated" | "unknown" | "missing";
  installHint: string | null;
}

export interface LocalRunnerSkill {
  id: string;
  name: string;
  filePath: string;
  description: string;
  tags: string[];
}

export interface LocalRunnerFlow {
  id: string;
  name: string;
  filePath: string;
  description: string;
  steps: string[];
}

export interface LocalRunnerPromptExecutionRequest {
  providerKey: string;
  prompt: string;
  skillIds: string[];
  flowId: string | null;
  contextSourceIds: string[];
  timeoutMs: number;
  workingDirectory: string | null;
}

export interface LocalRunnerPromptExecutionResult {
  status: "success" | "failed";
  runId: string;
  providerKey: string;
  command: string;
  stdoutSummary: string;
  stderrSummary: string;
  outputMarkdown: string;
  artifactPaths: string[];
  startedAt: string;
  completedAt: string;
  exitCode: number;
  errorMessage: string | null;
}
