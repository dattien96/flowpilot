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
