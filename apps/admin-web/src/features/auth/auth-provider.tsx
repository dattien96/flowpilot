import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

import { hasSupabaseEnv } from "@/lib/env/browser-env";
import { createSupabaseBrowserClient } from "@/data/supabase/client";

export interface AdminSession {
  user: {
    id: string;
    email: string | null;
  };
  mode: "supabase" | "demo";
}

interface AuthContextValue {
  session: AdminSession | null;
  loading: boolean;
  refreshSession: () => Promise<void>;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

async function readSession(): Promise<AdminSession | null> {
  if (!hasSupabaseEnv()) {
    return {
      mode: "demo",
      user: {
        id: "demo-user",
        email: "demo@flowpilot.local",
      },
    };
  }

  const supabase = createSupabaseBrowserClient();
  const { data, error } = await supabase.auth.getUser();

  if (error || !data.user) {
    return null;
  }

  return {
    mode: "supabase",
    user: {
      id: data.user.id,
      email: data.user.email ?? null,
    },
  };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<AdminSession | null>(null);
  const [loading, setLoading] = useState(true);

  const refreshSession = async () => {
    setSession(await readSession());
    setLoading(false);
  };

  useEffect(() => {
    let mounted = true;

    void readSession().then((nextSession) => {
      if (!mounted) return;
      setSession(nextSession);
      setLoading(false);
    });

    if (!hasSupabaseEnv()) {
      return () => {
        mounted = false;
      };
    }

    const supabase = createSupabaseBrowserClient();
    const {
      data: { subscription },
    } = supabase.auth.onAuthStateChange(() => {
      void readSession().then((nextSession) => {
        if (!mounted) return;
        setSession(nextSession);
      });
    });

    return () => {
      mounted = false;
      subscription.unsubscribe();
    };
  }, []);

  const signOut = async () => {
    if (!hasSupabaseEnv()) {
      return;
    }

    await createSupabaseBrowserClient().auth.signOut();
    setSession(null);
  };

  const value = useMemo<AuthContextValue>(
    () => ({
      session,
      loading,
      refreshSession,
      signOut,
    }),
    [loading, session],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);

  if (!value) {
    throw new Error("useAuth must be used within AuthProvider.");
  }

  return value;
}

