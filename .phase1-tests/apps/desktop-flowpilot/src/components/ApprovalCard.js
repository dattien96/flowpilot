"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ApprovalCard = ApprovalCard;
const jsx_runtime_1 = require("react/jsx-runtime");
const store_1 = require("@/state/store");
// Renders a permission_required event. In Part B this round-trips through the
// runner approval bridge (04-04); here it resolves the mock gate.
function ApprovalCard({ approvalId, details, decision }) {
    const approve = (0, store_1.useStore)((s) => s.approve);
    const resolved = decision !== undefined;
    return ((0, jsx_runtime_1.jsxs)("div", { className: `card approval ${resolved ? "resolved" : ""}`, children: [(0, jsx_runtime_1.jsxs)("div", { className: "card-head", children: [(0, jsx_runtime_1.jsx)("span", { className: "badge badge-warn", children: "Approval required" }), resolved && (0, jsx_runtime_1.jsxs)("span", { className: "badge", children: ["decision: ", decision] })] }), details.reason && (0, jsx_runtime_1.jsx)("p", { className: "card-reason", children: details.reason }), details.command && ((0, jsx_runtime_1.jsx)("pre", { className: "code-block", children: (0, jsx_runtime_1.jsx)("code", { children: details.command }) })), details.cwd && (0, jsx_runtime_1.jsxs)("div", { className: "meta", children: ["cwd: ", details.cwd] }), !resolved && ((0, jsx_runtime_1.jsx)("div", { className: "btn-row", children: details.decisions.map((d) => ((0, jsx_runtime_1.jsx)("button", { className: `btn ${d.value === "deny" ? "btn-danger" : "btn-primary"}`, onClick: () => void approve(approvalId, d.value), children: d.label }, d.value))) }))] }));
}
