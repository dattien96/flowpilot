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
      requestHttp(payload: {
        url: string;
        method?: string;
        headers?: Record<string, string>;
        body?: string;
      }): Promise<{
        status: number;
        headers: Array<[string, string]>;
        body: string;
      }>;
    };
  }
}
