import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  type ReactNode,
} from "react";

import { supabase } from "@/data/supabase/client";
import {
  clearAuthSession,
  getDemoSession,
  type AdminSession,
  setAuthLoading,
  setAuthSession,
  useAuthStore,
} from "@/features/auth/use-auth";
import { hasSupabaseEnv } from "@/lib/env/browser-env";

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
    if (!hasSupabaseEnv()) {
      return getDemoSession();
    }

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

    void (async () => {
      const nextSession = await readSession();
      if (!active) {
        return;
      }
      setAuthSession(nextSession);
    })();

    if (!hasSupabaseEnv()) {
      return () => {
        active = false;
      };
    }

    const {
      data: { subscription },
    } = supabase.auth.onAuthStateChange((_event, session) => {
      if (!active) {
        return;
      }
      setAuthSession(mapSupabaseSession(session));
    });

    return () => {
      active = false;
      subscription.unsubscribe();
    };
  }, []);

  const signOut = async () => {
    if (!hasSupabaseEnv()) {
      clearAuthSession();
      return;
    }

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
