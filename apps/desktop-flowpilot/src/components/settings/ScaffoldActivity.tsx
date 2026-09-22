import React, { useEffect, useRef, useState } from "react";
import {
  applyScaffoldSnapshot,
  emptyScaffoldFeed,
  fetchScaffoldProgress,
  scaffoldFeedTerminal,
  type ScaffoldFeedState,
} from "./projectEngine";

// CA-916: renders the runner's scaffold progress feed like a chat run — a live
// "AI Scaffold" card with phase milestones plus the provider's streamed stdout.
// Mounted wherever a scaffold turn can start (project create, engine init) and
// self-hides when the project has no scaffold activity.

const POLL_MS = 1200;
// How many empty, inactive polls to allow before concluding no scaffold will
// ever appear for this project (the create-project auto-trigger needs a few
// seconds to claim and emit its first event).
const IDLE_POLLS_BEFORE_HIDE = 6;

interface Props {
  projectId: string | null | undefined;
}

export function ScaffoldActivity({ projectId }: Props): React.ReactElement | null {
  const [feed, setFeed] = useState<ScaffoldFeedState>(emptyScaffoldFeed);
  const outputRef = useRef<HTMLPreElement | null>(null);

  useEffect(() => {
    if (!projectId) {
      setFeed(emptyScaffoldFeed);
      return;
    }
    const ctrl = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let idlePolls = 0;
    // feedRef mirrors the latest state inside the async loop without resubscribing.
    const feedRef = { current: emptyScaffoldFeed };
    const tick = async () => {
      try {
        const snap = await fetchScaffoldProgress(projectId, feedRef.current.cursor, ctrl.signal);
        feedRef.current = applyScaffoldSnapshot(feedRef.current, snap);
        setFeed(feedRef.current);
      } catch {
        // Runner down or transient: keep polling while a turn could be live.
      }
      if (ctrl.signal.aborted) return;
      if (scaffoldFeedTerminal(feedRef.current)) return;
      if (!feedRef.current.seen && ++idlePolls >= IDLE_POLLS_BEFORE_HIDE) {
        setFeed(emptyScaffoldFeed);
        return;
      }
      timer = setTimeout(() => void tick(), POLL_MS);
    };
    void tick();
    return () => {
      ctrl.abort();
      if (timer) clearTimeout(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  useEffect(() => {
    const el = outputRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [feed.output]);

  if (!feed.seen) return null;

  const statusLabel = feed.active
    ? `running — ${feed.phase || "starting"}${feed.attempt > 1 ? ` (attempt ${feed.attempt})` : ""}`
    : feed.result
      ? feed.result.status
      : feed.phase || "idle";

  return (
    <div className="project-inline-card" data-testid="scaffold-activity">
      <strong>
        AI Scaffold{" "}
        <span className={feed.active ? "settings-badge active" : "settings-badge"}>{statusLabel}</span>
      </strong>
      {feed.milestones.length > 0 ? (
        <div className="settings-list" style={{ marginTop: 8 }}>
          {feed.milestones.map((line, i) => (
            <div className="settings-list-item static" key={i}>
              <div><span>{line}</span></div>
            </div>
          ))}
        </div>
      ) : null}
      {feed.output ? (
        <pre
          ref={outputRef}
          className="scaffold-activity-output"
          style={{
            marginTop: 8,
            maxHeight: 220,
            overflowY: "auto",
            whiteSpace: "pre-wrap",
            fontSize: 12,
            padding: 8,
            borderRadius: 6,
            background: "var(--surface-2, rgba(0,0,0,0.25))",
          }}
        >
          {feed.output}
        </pre>
      ) : feed.active ? (
        <div className="settings-feedback" style={{ marginTop: 8 }}>
          Waiting for AI output…
        </div>
      ) : null}
      {feed.result?.message ? (
        <div className="settings-feedback" style={{ marginTop: 8 }}>{feed.result.message}</div>
      ) : null}
    </div>
  );
}
