import { useEffect, useState } from "react";
import { useStore } from "@/state/store";
import type { LSPStatus } from "@/types/contract";

// LSPStatusNotice: compact missing-language-server warning pinned at the top
// of the right sidebar stack (CP-63 follow-up). Null unless the bound
// project's platform has a registered server binary missing. At most two
// visual rows with CSS ellipsis, so it never pushes panels out of view.
// Data comes from GET /client/lsp-status (shared with the TUI sidebar);
// all logic lives server-side, this component only renders.
export function LSPStatusNotice(): React.ReactElement | null {
  const client = useStore((s) => s.client);
  const projects = useStore((s) => s.projects);
  const selectedProjectId = useStore((s) => s.selectedProjectId);
  const [status, setStatus] = useState<LSPStatus | null>(null);

  const cwd = projects.find((project) => project.id === selectedProjectId)?.path;
  useEffect(() => {
    if (!cwd) {
      setStatus(null);
      return;
    }
    let active = true;
    void (async () => {
      try {
        const next = await client.getLSPStatus?.(cwd);
        if (active) setStatus(next ?? null);
      } catch {
        if (active) setStatus(null);
      }
    })();
    return () => {
      active = false;
    };
  }, [client, cwd]);

  if (!status?.warn || !status.binary) return null;
  return (
    <section className="lsp-status-notice" aria-label="Language server status">
      <div className="lsp-status-title">⚠ LSP: {status.binary} missing</div>
      {status.installHint ? (
        <div className="lsp-status-hint" title={status.installHint}>
          → {status.installHint}
        </div>
      ) : null}
    </section>
  );
}
