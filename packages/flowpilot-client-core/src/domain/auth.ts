import type { RuntimeConfigRepository, SupabaseRuntimeStatus } from "./runtime";

export interface AuthSession {
  userId: string;
  email: string | null;
}

export interface AuthRepository {
  getSession(): Promise<AuthSession | null>;
  loginWithPassword(email: string, password: string): Promise<AuthSession>;
  logout(): Promise<void>;
  resetSessionState(): Promise<void> | void;
}

export interface DesktopBootstrapState {
  runtimeStatus: SupabaseRuntimeStatus;
  session: AuthSession | null;
}

export class LoadDesktopBootstrapUseCase {
  constructor(
    private readonly authRepository: AuthRepository,
    private readonly runtimeConfigRepository: RuntimeConfigRepository,
  ) {}

  async execute(): Promise<DesktopBootstrapState> {
    const runtimeStatus = await this.runtimeConfigRepository.loadSupabaseRuntimeStatus();
    const session = runtimeStatus.configured
      ? await this.authRepository.getSession()
      : null;
    return { runtimeStatus, session };
  }
}

export class LoginUseCase {
  constructor(private readonly authRepository: AuthRepository) {}

  execute(email: string, password: string): Promise<AuthSession> {
    return this.authRepository.loginWithPassword(email, password);
  }
}

export class LogoutUseCase {
  constructor(private readonly authRepository: AuthRepository) {}

  execute(): Promise<void> {
    return this.authRepository.logout();
  }
}

export class ResetAuthSessionStateUseCase {
  constructor(private readonly authRepository: AuthRepository) {}

  execute(): Promise<void> {
    return Promise.resolve(this.authRepository.resetSessionState());
  }
}
