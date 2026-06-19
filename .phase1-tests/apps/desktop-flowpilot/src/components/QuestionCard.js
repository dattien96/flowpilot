"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.QuestionCard = QuestionCard;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const store_1 = require("@/state/store");
const questionAnswer_1 = require("./questionAnswer");
const valueOf = (o) => o.value ?? o.label;
// The "popup with options" UX (the AskUserQuestion-style card). Backed in Part B
// by the user-interaction bridge (04-04) — both the model-driven `ask_user` MCP
// tool path and the deterministic workflow-driven path render THIS same card.
function QuestionCard({ prompt, options, multiSelect, answer }) {
    const submit = (0, store_1.useStore)((s) => s.answer);
    const resolved = answer !== undefined;
    const [selected, setSelected] = (0, react_1.useState)([]);
    const [other, setOther] = (0, react_1.useState)("");
    const toggle = (value) => {
        if (multiSelect) {
            setSelected((prev) => (prev.includes(value) ? prev.filter((v) => v !== value) : [...prev, value]));
        }
    };
    const answerOption = (value) => {
        if (!multiSelect) {
            void submit(value);
            return;
        }
        toggle(value);
    };
    const onSubmit = () => {
        const answer = (0, questionAnswer_1.resolveQuestionManualSubmit)(selected, other, multiSelect);
        if (answer === undefined)
            return;
        void submit(answer);
    };
    return ((0, jsx_runtime_1.jsxs)("div", { className: `card question ${resolved ? "resolved" : ""}`, children: [(0, jsx_runtime_1.jsxs)("div", { className: "card-head", children: [(0, jsx_runtime_1.jsx)("span", { className: "badge badge-ask", children: "Question" }), resolved && (0, jsx_runtime_1.jsx)("span", { className: "badge", children: "answered" })] }), (0, jsx_runtime_1.jsx)("p", { className: "card-prompt", children: prompt }), resolved ? ((0, jsx_runtime_1.jsxs)("div", { className: "meta", children: ["answer: ", Array.isArray(answer) ? answer.join(", ") : answer] })) : ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)("div", { className: "option-list", children: options.map((o) => {
                            const value = valueOf(o);
                            const active = selected.includes(value);
                            return ((0, jsx_runtime_1.jsxs)("button", { className: `option ${active ? "option-active" : ""}`, onClick: () => answerOption(value), children: [(0, jsx_runtime_1.jsx)("span", { className: "option-marker", children: multiSelect ? (active ? "☑" : "☐") : active ? "◉" : "○" }), (0, jsx_runtime_1.jsxs)("span", { className: "option-body", children: [(0, jsx_runtime_1.jsx)("span", { className: "option-label", children: o.label }), o.description && (0, jsx_runtime_1.jsx)("span", { className: "option-desc", children: o.description })] })] }, value));
                        }) }), (0, jsx_runtime_1.jsxs)("div", { className: "other-row", children: [(0, jsx_runtime_1.jsx)("input", { className: "text-input", placeholder: "Other\u2026", value: other, onChange: (e) => setOther(e.target.value) }), (0, jsx_runtime_1.jsx)("button", { className: "btn btn-primary", onClick: onSubmit, children: "Submit" })] })] }))] }));
}
