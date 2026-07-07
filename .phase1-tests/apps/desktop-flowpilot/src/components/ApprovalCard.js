"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ApprovalCard = ApprovalCard;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const store_1 = require("@/state/store");
// Shell metacharacters that make a command "compound". Mirrors the runner's
// approvalCommandOperators (BUG-246): a compound command can never be remembered,
// so the "don't ask again" checkbox is hidden for it and each run re-asks.
const COMMAND_OPERATORS = ["&&", "||", "|", ";", "&", ">", "<", "`", "$(", "(", ")", "{", "}", "\n", "\r"];
function isCompoundCommand(command) {
    return COMMAND_OPERATORS.some((op) => command.includes(op));
}
function looksLikeSubcommand(tok) {
    return /^[A-Za-z][A-Za-z0-9_-]*$/.test(tok);
}
// Wrapper tokens the runner skips when deriving a rule (mirrors approvalWrapperCommands).
const WRAPPER_COMMANDS = new Set(["rtk", "sudo", "time", "nice", "npx", "xargs"]);
// deriveApprovalRule mirrors the runner's granularity-B rule (leading wrappers
// skipped, then executable + subcommand) purely for the checkbox preview; the
// runner re-derives the authoritative rule when it persists.
function deriveApprovalRule(command) {
    const trimmed = command.trim();
    if (!trimmed || isCompoundCommand(trimmed))
        return null;
    const tokens = trimmed.split(/\s+/);
    if (tokens.length === 0)
        return null;
    let start = 0;
    while (start < tokens.length && WRAPPER_COMMANDS.has(tokens[start]))
        start++;
    if (start >= tokens.length)
        return tokens.join(" ");
    let end = start + 1;
    if (start + 1 < tokens.length && looksLikeSubcommand(tokens[start + 1]))
        end = start + 2;
    return tokens.slice(0, end).join(" ");
}
// Renders a permission_required event. In Part B this round-trips through the
// runner approval bridge (04-04); here it resolves the mock gate.
function ApprovalCard({ approvalId, details, decision }) {
    const approve = (0, store_1.useStore)((s) => s.approve);
    const [remember, setRemember] = (0, react_1.useState)(false);
    const resolved = decision !== undefined;
    // "Don't ask again" is offered only for shell commands (kind === "exec") that
    // are single (non-compound) — those are the only ones the runner will persist.
    const rule = details.kind === "exec" && details.command ? deriveApprovalRule(details.command) : null;
    const rememberable = !resolved && rule !== null;
    return ((0, jsx_runtime_1.jsxs)("div", { className: `card approval ${resolved ? "resolved" : ""}`, children: [(0, jsx_runtime_1.jsxs)("div", { className: "card-head", children: [(0, jsx_runtime_1.jsx)("span", { className: "badge badge-warn", children: "Approval required" }), resolved && (0, jsx_runtime_1.jsxs)("span", { className: "badge", children: ["decision: ", decision] })] }), details.reason && (0, jsx_runtime_1.jsx)("p", { className: "card-reason", children: details.reason }), details.command && ((0, jsx_runtime_1.jsx)("pre", { className: "code-block", children: (0, jsx_runtime_1.jsx)("code", { children: details.command }) })), details.cwd && (0, jsx_runtime_1.jsxs)("div", { className: "meta", children: ["cwd: ", details.cwd] }), !resolved && ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [rememberable && ((0, jsx_runtime_1.jsxs)("label", { className: "approval-remember", children: [(0, jsx_runtime_1.jsx)("input", { type: "checkbox", checked: remember, onChange: (e) => setRemember(e.target.checked) }), (0, jsx_runtime_1.jsxs)("span", { children: ["Don't ask again for commands starting with ", (0, jsx_runtime_1.jsx)("code", { children: rule })] })] })), (0, jsx_runtime_1.jsx)("div", { className: "btn-row", children: details.decisions.map((d) => ((0, jsx_runtime_1.jsx)("button", { className: `btn ${d.value === "deny" ? "btn-danger" : "btn-primary"}`, 
                            // Only carry "remember" on an approve-type decision — never persist a deny.
                            onClick: () => void approve(approvalId, d.value, d.value !== "deny" && remember), children: d.label }, d.value))) })] }))] }));
}
