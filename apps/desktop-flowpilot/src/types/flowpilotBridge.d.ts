export {};

declare global {
  interface Window {
    flowpilot?: {
      openInIde(file: string, line?: number): Promise<{ ok: boolean; stub?: boolean }>;
      openExternal(url: string): Promise<{ ok: boolean }>;
      loadAuthSession(): Promise<{
        clientKey: string;
        accessToken: string;
        refreshToken: string;
        userId: string;
        email?: string | null;
      } | null>;
      saveAuthSession(payload: {
        clientKey: string;
        accessToken: string;
        refreshToken: string;
        userId: string;
        email?: string | null;
      }): Promise<{ ok: boolean }>;
      clearAuthSession(): Promise<{ ok: boolean }>;
    };
  }
}
