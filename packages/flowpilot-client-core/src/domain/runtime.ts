export type SupabaseRuntimeMode = "config" | "env" | "demo";

export interface SupabaseRuntimeStatus {
  mode: SupabaseRuntimeMode;
  configured: boolean;
  apiUrl: string | null;
  anonKey: string | null;
  edgeFunctionUrl: string | null;
  hasServiceRoleKey: boolean;
  projectRef: string | null;
  runnerReachable: boolean;
  envAvailable: boolean;
  savedConfigAvailable: boolean;
  edgeFunctionsReady: boolean;
  lastError: string | null;
}

export interface SupabaseConfigInput {
  apiUrl: string;
  anonKey: string;
  edgeFunctionUrl: string;
  serviceRoleKey: string;
}

export interface ValidationCheck {
  key: string;
  status: "passed" | "failed" | "skipped";
  message: string;
}

export interface SupabaseConfigValidation {
  valid: boolean;
  checks: ValidationCheck[];
  projectRef?: string;
}

export interface SupabaseSchemaApplyInput {
  apiUrl: string;
  projectRef?: string;
  accessToken: string;
}

export interface SupabaseSchemaApplyMigration {
  version: string;
  name: string;
  status: "applied" | "skipped";
  message?: string;
}

export interface SupabaseSchemaApplyResult {
  projectRef: string;
  appliedCount: number;
  skippedCount: number;
  migrations: SupabaseSchemaApplyMigration[];
}

export interface RuntimeConfigRepository {
  loadSupabaseRuntimeStatus(): Promise<SupabaseRuntimeStatus>;
  validateSupabaseConfig(input: SupabaseConfigInput): Promise<SupabaseConfigValidation>;
  saveSupabaseConfig(input: SupabaseConfigInput): Promise<SupabaseRuntimeStatus>;
  applySupabaseMigrations(input: SupabaseSchemaApplyInput): Promise<SupabaseSchemaApplyResult>;
}

export class LoadSupabaseRuntimeStatusUseCase {
  constructor(private readonly repository: RuntimeConfigRepository) {}

  execute(): Promise<SupabaseRuntimeStatus> {
    return this.repository.loadSupabaseRuntimeStatus();
  }
}

export class ValidateSupabaseConfigUseCase {
  constructor(private readonly repository: RuntimeConfigRepository) {}

  execute(input: SupabaseConfigInput): Promise<SupabaseConfigValidation> {
    return this.repository.validateSupabaseConfig(input);
  }
}

export class SaveSupabaseConfigUseCase {
  constructor(private readonly repository: RuntimeConfigRepository) {}

  execute(input: SupabaseConfigInput): Promise<SupabaseRuntimeStatus> {
    return this.repository.saveSupabaseConfig(input);
  }
}

export class ApplySupabaseMigrationsUseCase {
  constructor(private readonly repository: RuntimeConfigRepository) {}

  execute(input: SupabaseSchemaApplyInput): Promise<SupabaseSchemaApplyResult> {
    return this.repository.applySupabaseMigrations(input);
  }
}
