import type { ApprovalDetails } from "@/types/contract";
import { useState } from "react";
import { useStore } from "@/state/store";

interface Props {
  approvalId: string;
  details: ApprovalDetails;
  decision?: string;
}

// Shell metacharacters that make a command "compound". Mirrors the runner's
// approvalCommandOperators (BUG-246): a compound command can never be remembered,
// so the "don't ask again" checkbox is hidden for it and each run re-asks.
const COMMAND_OPERATORS = ["&&", "||", "|", ";", "&", ">", "<", "`", "$(", "(", ")", "{", "}", "\n", "\r"];

function isCompoundCommand(command: string): boolean {
  return COMMAND_OPERATORS.some((op) => command.includes(op));
}

function looksLikeSubcommand(tok: string): boolean {
  return /^[A-Za-z][A-Za-z0-9_-]*$/.test(tok);
}

// Wrapper tokens the runner skips when deriving a rule (mirrors approvalWrapperCommands).
const WRAPPER_COMMANDS = new Set(["rtk", "sudo", "time", "nice", "npx", "xargs"]);

// deriveApprovalRule mirrors the runner's granularity-B rule (leading wrappers
// skipped, then executable + subcommand) purely for the checkbox preview; the
// runner re-derives the authoritative rule when it persists.
function deriveApprovalRule(command: string): string | null {
  const trimmed = command.trim();
  if (!trimmed || isCompoundCommand(trimmed)) return null;
  const tokens = trimmed.split(/\s+/);
  if (tokens.length === 0) return null;
  let start = 0;
  while (start < tokens.length && WRAPPER_COMMANDS.has(tokens[start])) start++;
  if (start >= tokens.length) return tokens.join(" ");
  let end = start + 1;
  if (start + 1 < tokens.length && looksLikeSubcommand(tokens[start + 1])) end = start + 2;
  return tokens.slice(0, end).join(" ");
}

// Renders a permission_required event. In Part B this round-trips through the
// runner approval bridge (04-04); here it resolves the mock gate.
export function ApprovalCard({ approvalId, details, decision }: Props): React.ReactElement {
  const approve = useStore((s) => s.approve);
  const [remember, setRemember] = useState(false);
  const resolved = decision !== undefined;

  // "Don't ask again" is offered only for shell commands (kind === "exec") that
  // are single (non-compound) — those are the only ones the runner will persist.
  const rule = details.kind === "exec" && details.command ? deriveApprovalRule(details.command) : null;
  const rememberable = !resolved && rule !== null;

  return (
    <div className={`card approval ${resolved ? "resolved" : ""}`}>
      <div className="card-head">
        <span className="badge badge-warn">Approval required</span>
        {resolved && <span className="badge">decision: {decision}</span>}
      </div>
      {details.reason && <p className="card-reason">{details.reason}</p>}
      {details.command && (
        <pre className="code-block">
          <code>{details.command}</code>
        </pre>
      )}
      {details.cwd && <div className="meta">cwd: {details.cwd}</div>}
      {!resolved && (
        <>
          {rememberable && (
            <label className="approval-remember">
              <input
                type="checkbox"
                checked={remember}
                onChange={(e) => setRemember(e.target.checked)}
              />
              <span>
                Don't ask again for commands starting with <code>{rule}</code>
              </span>
            </label>
          )}
          <div className="btn-row">
            {details.decisions.map((d) => (
              <button
                key={d.value}
                className={`btn ${d.value === "deny" ? "btn-danger" : "btn-primary"}`}
                // Only carry "remember" on an approve-type decision — never persist a deny.
                onClick={() => void approve(approvalId, d.value, d.value !== "deny" && remember)}
              >
                {d.label}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
