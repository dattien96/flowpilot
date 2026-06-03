import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  type ReactNode,
} from "react";

import { getBrowserSupabaseClient } from "@/data/supabase/client";
import {
  clearAuthSession,
  getDemoSession,
  type AdminSession,
  setAuthLoading,
  setAuthSession,
  useAuthStore,
} from "@/features/auth/use-auth";
import { loadSupabaseRuntimeStatus } from "@/lib/supabase/runtime-config";

interface AuthContextValue {
  loading: boolean;
  refreshSession: () => Promise<void>;
  session: AdminSession | null;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

function mapSupabaseSession(
  session:
    | {
        user?: {
          email?: string | null;
          id: string;
        } | null;
      }
    | null,
): AdminSession | null {
  if (!session?.user) {
    return null;
  }

  return {
    mode: "supabase",
    user: {
      email: session.user.email ?? null,
      id: session.user.id,
    },
  };
}

async function readSession(): Promise<AdminSession | null> {
  try {
    const status = await loadSupabaseRuntimeStatus();
    if (!status.configured) {
      return getDemoSession();
    }

    const supabase = await getBrowserSupabaseClient();
    const {
      data: { session },
    } = await supabase.auth.getSession();

    return mapSupabaseSession(session);
  } catch (error) {
    console.error("Failed to read the current admin session.", error);
    return null;
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const loading = useAuthStore((state) => state.loading);
  const session = useAuthStore((state) => state.session);

  const refreshSession = async () => {
    setAuthLoading(true);
    setAuthSession(await readSession());
  };

  useEffect(() => {
    let active = true;
    let cleanup: (() => void) | null = null;

    void (async () => {
      const status = await loadSupabaseRuntimeStatus();
      const nextSession = await readSession();
      if (!active) {
        return;
      }
      setAuthSession(nextSession);

      if (!status.configured) {
        return;
      }

      const supabase = await getBrowserSupabaseClient();
      if (!active) {
        return;
      }
      const {
        data: { subscription },
      } = supabase.auth.onAuthStateChange((_event, session) => {
        if (!active) {
          return;
        }
        setAuthSession(mapSupabaseSession(session));
      });

      cleanup = () => subscription.unsubscribe();
    })();

    return () => {
      active = false;
      cleanup?.();
    };
  }, []);

  const signOut = async () => {
    const status = await loadSupabaseRuntimeStatus();
    if (!status.configured) {
      clearAuthSession();
      return;
    }

    const supabase = await getBrowserSupabaseClient();
    await supabase.auth.signOut();
    clearAuthSession();
  };

  const value = useMemo<AuthContextValue>(
    () => ({
      loading,
      refreshSession,
      session,
      signOut,
    }),
    [loading, session],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);

  if (!context) {
    throw new Error("useAuth must be used within AuthProvider.");
  }

  return context;
}
