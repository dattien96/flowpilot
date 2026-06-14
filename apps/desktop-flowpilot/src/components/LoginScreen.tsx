import { useState } from "react";
import type { SupabaseRuntimeStatus } from "@flowpilot/client-core";

interface LoginScreenProps {
  busy: boolean;
  runtimeStatus: SupabaseRuntimeStatus;
  onLogin: (email: string, password: string) => Promise<void>;
  onOpenSettings: () => void;
}

export function LoginScreen({
  busy,
  runtimeStatus,
  onLogin,
  onOpenSettings,
}: LoginScreenProps): React.ReactElement {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const configured = runtimeStatus.configured;

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy) return;
    setErrorMessage(null);
    try {
      await onLogin(email, password);
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "Unable to sign in.");
    }
  };

  return (
    <div className="auth-shell">
      <div className="auth-card">
        <p className="auth-kicker">FlowPilot Desktop</p>
        <h1 className="auth-title">Sign in</h1>
        <p className="auth-copy">
          Sign in to the active workspace Supabase project or open settings for this
          machine first.
        </p>

        {!configured ? (
          <div className="auth-warning">
            Supabase database is not configured for this PC.
            {runtimeStatus.lastError ? <span>{runtimeStatus.lastError}</span> : null}
          </div>
        ) : null}

        <form className="auth-form" onSubmit={handleSubmit}>
          <label className="auth-field">
            <span>Email</span>
            <input
              disabled={busy || !configured}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="admin@flowpilot.local"
              type="email"
              value={email}
            />
          </label>

          <label className="auth-field">
            <span>Password</span>
            <input
              disabled={busy || !configured}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="Password"
              type="password"
              value={password}
            />
          </label>

          {errorMessage ? <div className="auth-error">{errorMessage}</div> : null}

          <div className="auth-actions">
            <button className="primary-btn" disabled={busy || !configured} type="submit">
              {busy ? "Signing in..." : "Sign in"}
            </button>
            <button className="secondary-btn" onClick={onOpenSettings} type="button">
              Database Settings
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
