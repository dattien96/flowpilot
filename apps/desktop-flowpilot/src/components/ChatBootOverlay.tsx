import { useStore, type ChatBootStep } from "@/state/store";
import { WarnIcon } from "@/components/icons";

const BOOT_STEPS: { key: ChatBootStep; label: string }[] = [
  { key: "projects", label: "Connect to runner & load projects" },
  { key: "catalog", label: "Load providers & models" },
  { key: "accounts", label: "Load accounts & skills" },
  { key: "session", label: "Restore chat session" },
];

/**
 * Scoped loading gate for the chat workspace's main column. Covers only the
 * chat area — header tabs (Settings), sidebars (incl. system controls), and
 * the terminal dock stay interactive while the first loadProjects() pipeline
 * finishes. Chat content stays mounted underneath so drafts/timeline state
 * and provider fallback effects settle before the overlay lifts.
 */
export function ChatBootOverlay(): React.ReactElement | null {
  const chatBoot = useStore((s) => s.chatBoot);
  const loadProjects = useStore((s) => s.loadProjects);

  if (chatBoot.status === "ready") return null;

  const failed = chatBoot.status === "failed";
  const activeIndex = Math.max(
    BOOT_STEPS.findIndex((s) => s.key === chatBoot.step),
    0,
  );

  return (
    <div className="chat-boot-overlay" aria-busy="true" aria-label="Preparing chat workspace">
      <div className="chat-boot-card">
        {failed ? (
          <>
            <p className="chat-boot-title">
              <span className="chat-boot-title-icon err" aria-hidden="true">
                <WarnIcon size={15} />
              </span>
              Couldn&apos;t prepare the workspace
            </p>
            <p className="chat-boot-error">{chatBoot.error}</p>
            <div className="chat-boot-actions">
              <button
                type="button"
                className="project-history-confirm-ok"
                onClick={() => void loadProjects()}
              >
                Retry
              </button>
            </div>
          </>
        ) : (
          <>
            <p className="chat-boot-title">
              <span className="history-status-spinner chat-boot-title-icon" aria-hidden="true" />
              Preparing chat workspace
            </p>
            <ul className="chat-boot-steps">
              {BOOT_STEPS.map((step, index) => {
                const state =
                  index < activeIndex ? "done" : index === activeIndex ? "active" : "pending";
                return (
                  <li key={step.key} className={`chat-boot-step ${state}`}>
                    <span className="chat-boot-step-icon" aria-hidden="true">
                      {state === "done" ? "✓" : state === "active" ? (
                        <span className="history-status-spinner" />
                      ) : (
                        "·"
                      )}
                    </span>
                    {step.label}
                  </li>
                );
              })}
            </ul>
            <p className="chat-boot-hint">
              Settings and system controls stay available while this loads.
            </p>
          </>
        )}
      </div>
    </div>
  );
}
