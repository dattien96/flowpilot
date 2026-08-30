"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.QuestionCard = QuestionCard;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const createRunnerClient_1 = require("@/client/createRunnerClient");
const store_1 = require("@/state/store");
const questionAnswer_1 = require("./questionAnswer");
const valueOf = (o) => o.value ?? o.label;
const GOOGLE_DRIVE_PICKER_OPTION = "__google_drive_picker__";
// The "popup with options" UX (the AskUserQuestion-style card). Backed in Part B
// by the user-interaction bridge (04-04) — both the model-driven `ask_user` MCP
// tool path and the deterministic workflow-driven path render THIS same card.
function QuestionCard({ questionId, prompt, options, multiSelect, answer }) {
    const submit = (0, store_1.useStore)((s) => s.answer);
    const resolved = answer !== undefined;
    const [selected, setSelected] = (0, react_1.useState)([]);
    const [other, setOther] = (0, react_1.useState)("");
    const pickerMessageCleanupRef = (0, react_1.useRef)(null);
    (0, react_1.useEffect)(() => () => {
        pickerMessageCleanupRef.current?.();
        pickerMessageCleanupRef.current = null;
    }, []);
    const openGoogleDrivePicker = () => {
        const base = (0, createRunnerClient_1.getRunnerBaseUrl)();
        if (!base)
            return;
        const popup = window.open(`${base}/client/questions/${encodeURIComponent(questionId)}/google-drive-picker`, "_blank", "width=980,height=820");
        if (!popup)
            return;
        const handleMessage = (event) => {
            if (event.data?.type !== "flowpilot-google-drive-question-picked")
                return;
            if (event.data?.questionId !== questionId || typeof event.data?.choice !== "string")
                return;
            window.removeEventListener("message", handleMessage);
            pickerMessageCleanupRef.current = null;
            void submit(questionId, event.data.choice);
        };
        pickerMessageCleanupRef.current?.();
        window.addEventListener("message", handleMessage);
        pickerMessageCleanupRef.current = () => window.removeEventListener("message", handleMessage);
    };
    const toggle = (value) => {
        if (multiSelect) {
            setSelected((prev) => (prev.includes(value) ? prev.filter((v) => v !== value) : [...prev, value]));
        }
    };
    const answerOption = (value) => {
        if (value === GOOGLE_DRIVE_PICKER_OPTION) {
            openGoogleDrivePicker();
            return;
        }
        if (!multiSelect) {
            void submit(questionId, value);
            return;
        }
        toggle(value);
    };
    const onSubmit = () => {
        const answer = (0, questionAnswer_1.resolveQuestionManualSubmit)(selected, other, multiSelect);
        if (answer === undefined)
            return;
        void submit(questionId, answer);
    };
    return ((0, jsx_runtime_1.jsxs)("div", { className: `card question ${resolved ? "resolved" : ""}`, children: [(0, jsx_runtime_1.jsxs)("div", { className: "card-head", children: [(0, jsx_runtime_1.jsx)("span", { className: "badge badge-ask", children: "Question" }), resolved && (0, jsx_runtime_1.jsx)("span", { className: "badge", children: "answered" })] }), (0, jsx_runtime_1.jsx)("p", { className: "card-prompt", children: prompt }), resolved ? ((0, jsx_runtime_1.jsxs)("div", { className: "meta", children: ["answer: ", Array.isArray(answer) ? answer.join(", ") : answer] })) : ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)("div", { className: "option-list", children: options.map((o) => {
                            const value = valueOf(o);
                            const active = selected.includes(value);
                            return ((0, jsx_runtime_1.jsxs)("button", { className: `option ${active ? "option-active" : ""}`, onClick: () => answerOption(value), children: [(0, jsx_runtime_1.jsx)("span", { className: "option-marker", children: multiSelect ? (active ? "☑" : "☐") : active ? "◉" : "○" }), (0, jsx_runtime_1.jsxs)("span", { className: "option-body", children: [(0, jsx_runtime_1.jsx)("span", { className: "option-label", children: o.label }), o.description && (0, jsx_runtime_1.jsx)("span", { className: "option-desc", children: o.description })] })] }, value));
                        }) }), (0, jsx_runtime_1.jsxs)("div", { className: "other-row", children: [(0, jsx_runtime_1.jsx)("input", { className: "text-input", placeholder: "Other\u2026", value: other, onChange: (e) => setOther(e.target.value) }), (0, jsx_runtime_1.jsx)("button", { className: "btn btn-primary", onClick: onSubmit, children: "Submit" })] })] }))] }));
}
