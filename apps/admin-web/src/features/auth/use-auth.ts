import { create } from "zustand";

export interface AdminSession {
  mode: "demo" | "supabase";
  user: {
    email: string | null;
    id: string;
  };
}

interface AuthState {
  loading: boolean;
  session: AdminSession | null;
  setLoading: (loading: boolean) => void;
  setSession: (session: AdminSession | null) => void;
}

export function getDemoSession(): AdminSession {
  return {
    mode: "demo",
    user: {
      email: "demo@flowpilot.local",
      id: "demo-user",
    },
  };
}

export const useAuthStore = create<AuthState>((set) => ({
  loading: true,
  session: null,
  setLoading: (loading) => set({ loading }),
  setSession: (session) => set({ loading: false, session }),
}));

export function setAuthLoading(loading: boolean) {
  useAuthStore.getState().setLoading(loading);
}

export function setAuthSession(session: AdminSession | null) {
  useAuthStore.getState().setSession(session);
}

export function clearAuthSession() {
  useAuthStore.getState().setSession(null);
}
