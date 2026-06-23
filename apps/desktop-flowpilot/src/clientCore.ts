import {
  AdminUseCases,
  ApplySupabaseMigrationsUseCase,
  CompositeArtifactRepository,
  CompositeIntegrationRepository,
  CompositeProviderRepository,
  HttpRunnerRepository,
  LoadCompatConfigUseCase,
  LoadCompatInfoUseCase,
  LoadDesktopBootstrapUseCase,
  LoadRunnerHealthUseCase,
  LoadSupabaseRuntimeStatusUseCase,
  LoginUseCase,
  LogoutUseCase,
  ResetAuthSessionStateUseCase,
  RunCompatCheckUseCase,
  RunCompatDeepCheckUseCase,
  SaveCompatConfigUseCase,
  RunnerRuntimeConfigRepository,
  RunnerAdminRepository,
  SaveSupabaseConfigUseCase,
  SupabaseAdminRepository,
  ValidateSupabaseConfigUseCase,
  fetchHttpClient,
} from "@flowpilot/client-core";
import { RUNNER_URL } from "@/config";
import { createDesktopAdminSupabaseClient } from "@/auth/desktopAdminSupabaseClient";
import { DesktopSupabaseAuthRepository } from "@/auth/desktopSupabaseAuthRepository";

export const runtimeConfigRepository = new RunnerRuntimeConfigRepository(
  fetchHttpClient,
  RUNNER_URL,
);

export const runnerRepository = new HttpRunnerRepository(fetchHttpClient, RUNNER_URL);

export const authRepository = new DesktopSupabaseAuthRepository(
  runtimeConfigRepository,
  fetchHttpClient,
  RUNNER_URL,
);

export const loadDesktopBootstrapUseCase = new LoadDesktopBootstrapUseCase(
  authRepository,
  runtimeConfigRepository,
);

export const loadSupabaseRuntimeStatusUseCase = new LoadSupabaseRuntimeStatusUseCase(
  runtimeConfigRepository,
);

export const validateSupabaseConfigUseCase = new ValidateSupabaseConfigUseCase(
  runtimeConfigRepository,
);

export const saveSupabaseConfigUseCase = new SaveSupabaseConfigUseCase(
  runtimeConfigRepository,
);

export const applySupabaseMigrationsUseCase = new ApplySupabaseMigrationsUseCase(
  runtimeConfigRepository,
);

export const loadRunnerHealthUseCase = new LoadRunnerHealthUseCase(runnerRepository);
export const loadCompatConfigUseCase = new LoadCompatConfigUseCase(runnerRepository);
export const loadCompatInfoUseCase = new LoadCompatInfoUseCase(runnerRepository);
export const runCompatCheckUseCase = new RunCompatCheckUseCase(runnerRepository);
export const runCompatDeepCheckUseCase = new RunCompatDeepCheckUseCase(runnerRepository);
export const saveCompatConfigUseCase = new SaveCompatConfigUseCase(runnerRepository);
export const loginUseCase = new LoginUseCase(authRepository);
export const logoutUseCase = new LogoutUseCase(authRepository);
export const resetAuthSessionStateUseCase = new ResetAuthSessionStateUseCase(authRepository);

let adminUseCasesPromise: Promise<AdminUseCases> | null = null;

export function resetAdminUseCases() {
  adminUseCasesPromise = null;
}

export function getAdminUseCases() {
  adminUseCasesPromise ??= createAdminUseCases();
  return adminUseCasesPromise;
}

async function createAdminUseCases() {
  const config = await runtimeConfigRepository.loadSupabaseWorkspaceConfigWithSecret();
  if (!config?.apiUrl || !config.anonKey) {
    throw new Error("Supabase database is not configured.");
  }

  const supabase = await createDesktopAdminSupabaseClient(
    config.apiUrl,
    config.serviceRoleKey ?? config.anonKey,
  );
  const supabaseRepository = new SupabaseAdminRepository(supabase);
  const runnerAdminRepository = new RunnerAdminRepository(fetchHttpClient, RUNNER_URL);

  return new AdminUseCases(
    supabaseRepository,
    supabaseRepository,
    supabaseRepository,
    new CompositeArtifactRepository(supabaseRepository, runnerAdminRepository),
    new CompositeProviderRepository(supabaseRepository, runnerAdminRepository),
    new CompositeIntegrationRepository(supabaseRepository, runnerAdminRepository),
    runnerAdminRepository,
  );
}
