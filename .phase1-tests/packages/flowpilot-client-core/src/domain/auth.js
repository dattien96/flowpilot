"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ResetAuthSessionStateUseCase = exports.LogoutUseCase = exports.LoginUseCase = exports.LoadDesktopBootstrapUseCase = void 0;
class LoadDesktopBootstrapUseCase {
    authRepository;
    runtimeConfigRepository;
    constructor(authRepository, runtimeConfigRepository) {
        this.authRepository = authRepository;
        this.runtimeConfigRepository = runtimeConfigRepository;
    }
    async execute() {
        const runtimeStatus = await this.runtimeConfigRepository.loadSupabaseRuntimeStatus();
        const session = runtimeStatus.configured
            ? await this.authRepository.getSession()
            : null;
        return { runtimeStatus, session };
    }
}
exports.LoadDesktopBootstrapUseCase = LoadDesktopBootstrapUseCase;
class LoginUseCase {
    authRepository;
    constructor(authRepository) {
        this.authRepository = authRepository;
    }
    execute(email, password) {
        return this.authRepository.loginWithPassword(email, password);
    }
}
exports.LoginUseCase = LoginUseCase;
class LogoutUseCase {
    authRepository;
    constructor(authRepository) {
        this.authRepository = authRepository;
    }
    execute() {
        return this.authRepository.logout();
    }
}
exports.LogoutUseCase = LogoutUseCase;
class ResetAuthSessionStateUseCase {
    authRepository;
    constructor(authRepository) {
        this.authRepository = authRepository;
    }
    execute() {
        return Promise.resolve(this.authRepository.resetSessionState());
    }
}
exports.ResetAuthSessionStateUseCase = ResetAuthSessionStateUseCase;
