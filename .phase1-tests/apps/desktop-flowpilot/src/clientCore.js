"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.resetAuthSessionStateUseCase = exports.logoutUseCase = exports.loginUseCase = exports.saveCompatConfigUseCase = exports.runCompatDeepCheckUseCase = exports.runCompatCheckUseCase = exports.loadCompatInfoUseCase = exports.loadCompatConfigUseCase = exports.loadRunnerHealthUseCase = exports.applySupabaseMigrationsUseCase = exports.saveSupabaseConfigUseCase = exports.validateSupabaseConfigUseCase = exports.loadSupabaseRuntimeStatusUseCase = exports.loadDesktopBootstrapUseCase = exports.authRepository = exports.runnerRepository = exports.runtimeConfigRepository = void 0;
exports.resetAdminUseCases = resetAdminUseCases;
exports.getAdminUseCases = getAdminUseCases;
const client_core_1 = require("@flowpilot/client-core");
const config_1 = require("@/config");
const desktopAdminSupabaseClient_1 = require("@/auth/desktopAdminSupabaseClient");
const desktopSupabaseAuthRepository_1 = require("@/auth/desktopSupabaseAuthRepository");
exports.runtimeConfigRepository = new client_core_1.RunnerRuntimeConfigRepository(client_core_1.fetchHttpClient, config_1.RUNNER_URL);
exports.runnerRepository = new client_core_1.HttpRunnerRepository(client_core_1.fetchHttpClient, config_1.RUNNER_URL);
exports.authRepository = new desktopSupabaseAuthRepository_1.DesktopSupabaseAuthRepository(exports.runtimeConfigRepository, client_core_1.fetchHttpClient, config_1.RUNNER_URL);
exports.loadDesktopBootstrapUseCase = new client_core_1.LoadDesktopBootstrapUseCase(exports.authRepository, exports.runtimeConfigRepository);
exports.loadSupabaseRuntimeStatusUseCase = new client_core_1.LoadSupabaseRuntimeStatusUseCase(exports.runtimeConfigRepository);
exports.validateSupabaseConfigUseCase = new client_core_1.ValidateSupabaseConfigUseCase(exports.runtimeConfigRepository);
exports.saveSupabaseConfigUseCase = new client_core_1.SaveSupabaseConfigUseCase(exports.runtimeConfigRepository);
exports.applySupabaseMigrationsUseCase = new client_core_1.ApplySupabaseMigrationsUseCase(exports.runtimeConfigRepository);
exports.loadRunnerHealthUseCase = new client_core_1.LoadRunnerHealthUseCase(exports.runnerRepository);
exports.loadCompatConfigUseCase = new client_core_1.LoadCompatConfigUseCase(exports.runnerRepository);
exports.loadCompatInfoUseCase = new client_core_1.LoadCompatInfoUseCase(exports.runnerRepository);
exports.runCompatCheckUseCase = new client_core_1.RunCompatCheckUseCase(exports.runnerRepository);
exports.runCompatDeepCheckUseCase = new client_core_1.RunCompatDeepCheckUseCase(exports.runnerRepository);
exports.saveCompatConfigUseCase = new client_core_1.SaveCompatConfigUseCase(exports.runnerRepository);
exports.loginUseCase = new client_core_1.LoginUseCase(exports.authRepository);
exports.logoutUseCase = new client_core_1.LogoutUseCase(exports.authRepository);
exports.resetAuthSessionStateUseCase = new client_core_1.ResetAuthSessionStateUseCase(exports.authRepository);
let adminUseCasesPromise = null;
function resetAdminUseCases() {
    adminUseCasesPromise = null;
}
function getAdminUseCases() {
    adminUseCasesPromise ??= createAdminUseCases();
    return adminUseCasesPromise;
}
async function createAdminUseCases() {
    const config = await exports.runtimeConfigRepository.loadSupabaseWorkspaceConfigWithSecret();
    if (!config?.apiUrl || !config.anonKey) {
        throw new Error("Supabase database is not configured.");
    }
    const supabase = await (0, desktopAdminSupabaseClient_1.createDesktopAdminSupabaseClient)(config.apiUrl, config.serviceRoleKey ?? config.anonKey);
    const supabaseRepository = new client_core_1.SupabaseAdminRepository(supabase);
    const runnerAdminRepository = new client_core_1.RunnerAdminRepository(client_core_1.fetchHttpClient, config_1.RUNNER_URL);
    return new client_core_1.AdminUseCases(supabaseRepository, supabaseRepository, supabaseRepository, new client_core_1.CompositeArtifactRepository(supabaseRepository, runnerAdminRepository), new client_core_1.CompositeProviderRepository(supabaseRepository, runnerAdminRepository), new client_core_1.CompositeIntegrationRepository(supabaseRepository, runnerAdminRepository), runnerAdminRepository);
}
