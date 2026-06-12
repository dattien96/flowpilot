import { useState } from "react";
import { useStore } from "@/state/store";
import { ADMIN_WEB_URL } from "@/config";

// Sidebar system controls. Mirrors the admin-web app-shell actions:
//  - Open Admin Web  → default browser at :3002
//  - Restart system  → runner POST /system/restart  (mock: stub)
//  - Shutdown system → runner POST /system/shutdown  (mock: stub)
export function SystemControls(): React.ReactElement {
  const openAdminWeb = useStore((s) => s.openAdminWeb);
  const restartSystem = useStore((s) => s.restartSystem);
  const shutdownSystem = useStore((s) => s.shutdownSystem);
  const [pending, setPending] = useState<"restart" | "shutdown" | null>(null);

  const onRestart = async () => {
    if (!window.confirm("Are you sure you want to restart the dev stack?")) return;
    setPending("restart");
    try {
      await restartSystem();
    } finally {
      setPending(null);
    }
  };

  const onShutdown = async () => {
    if (!window.confirm("Shut down the dev stack? Runner and web will stop.")) return;
    setPending("shutdown");
    try {
      await shutdownSystem();
    } finally {
      setPending(null);
    }
  };

  return (
    <div className="system-controls">
      <span className="sc-title">System</span>
      <button className="sc-btn" onClick={openAdminWeb} title={ADMIN_WEB_URL}>
        🌐 Open Admin Web
      </button>
      <button className="sc-btn" onClick={() => void onRestart()} disabled={pending !== null}>
        {pending === "restart" ? "↻ Restarting…" : "↻ Restart system"}
      </button>
      <button className="sc-btn sc-danger" onClick={() => void onShutdown()} disabled={pending !== null}>
        {pending === "shutdown" ? "⏻ Shutting down…" : "⏻ Turn off system"}
      </button>
    </div>
  );
}
