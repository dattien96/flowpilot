import { createClient } from "@supabase/supabase-js";
import {
  AdminUseCases,
  CompositeArtifactRepository,
  CompositeIntegrationRepository,
  CompositeProviderRepository,
  HttpRunnerRepository,
  LoadDesktopBootstrapUseCase,
  LoadRunnerHealthUseCase,
  LoadSupabaseRuntimeStatusUseCase,
  LoginUseCase,
  LogoutUseCase,
  ResetAuthSessionStateUseCase,
  RunnerRuntimeConfigRepository,
  RunnerAdminRepository,
  SaveSupabaseConfigUseCase,
  SupabaseAdminRepository,
  ValidateSupabaseConfigUseCase,
  fetchHttpClient,
} from "@flowpilot/client-core";
import { RUNNER_URL } from "@/config";
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

export const loadRunnerHealthUseCase = new LoadRunnerHealthUseCase(runnerRepository);
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
  const runtimeStatus = await runtimeConfigRepository.loadSupabaseRuntimeStatus();
  if (!runtimeStatus.configured || !runtimeStatus.apiUrl || !runtimeStatus.anonKey) {
    throw new Error("Supabase database is not configured.");
  }

  const supabase = createClient(runtimeStatus.apiUrl, runtimeStatus.anonKey);
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
