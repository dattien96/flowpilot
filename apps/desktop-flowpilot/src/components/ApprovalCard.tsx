import type { ApprovalDetails } from "@/types/contract";
import { useStore } from "@/state/store";

interface Props {
  approvalId: string;
  details: ApprovalDetails;
  decision?: string;
}

// Renders a permission_required event. In Part B this round-trips through the
// runner approval bridge (04-04); here it resolves the mock gate.
export function ApprovalCard({ approvalId, details, decision }: Props): React.ReactElement {
  const approve = useStore((s) => s.approve);
  const resolved = decision !== undefined;

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
        <div className="btn-row">
          {details.decisions.map((d) => (
            <button
              key={d.value}
              className={`btn ${d.value === "deny" ? "btn-danger" : "btn-primary"}`}
              onClick={() => void approve(approvalId, d.value)}
            >
              {d.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
