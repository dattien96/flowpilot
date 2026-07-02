import { useState } from "react";
import type { QuestionOption } from "@/types/contract";
import { useStore } from "@/state/store";
import { resolveQuestionManualSubmit } from "./questionAnswer";

interface Props {
  questionId: string;
  prompt: string;
  options: QuestionOption[];
  multiSelect?: boolean;
  answer?: string | string[];
}

const valueOf = (o: QuestionOption): string => o.value ?? o.label;

// The "popup with options" UX (the AskUserQuestion-style card). Backed in Part B
// by the user-interaction bridge (04-04) — both the model-driven `ask_user` MCP
// tool path and the deterministic workflow-driven path render THIS same card.
export function QuestionCard({ questionId, prompt, options, multiSelect, answer }: Props): React.ReactElement {
  const submit = useStore((s) => s.answer);
  const resolved = answer !== undefined;
  const [selected, setSelected] = useState<string[]>([]);
  const [other, setOther] = useState("");

  const toggle = (value: string) => {
    if (multiSelect) {
      setSelected((prev) => (prev.includes(value) ? prev.filter((v) => v !== value) : [...prev, value]));
    }
  };

  const answerOption = (value: string) => {
    if (!multiSelect) {
      void submit(questionId, value);
      return;
    }

    toggle(value);
  };

  const onSubmit = () => {
    const answer = resolveQuestionManualSubmit(selected, other, multiSelect);
    if (answer === undefined) return;
    void submit(questionId, answer);
  };

  return (
    <div className={`card question ${resolved ? "resolved" : ""}`}>
      <div className="card-head">
        <span className="badge badge-ask">Question</span>
        {resolved && <span className="badge">answered</span>}
      </div>
      <p className="card-prompt">{prompt}</p>

      {resolved ? (
        <div className="meta">answer: {Array.isArray(answer) ? answer.join(", ") : answer}</div>
      ) : (
        <>
          <div className="option-list">
            {options.map((o) => {
              const value = valueOf(o);
              const active = selected.includes(value);
              return (
                <button
                  key={value}
                  className={`option ${active ? "option-active" : ""}`}
                  onClick={() => answerOption(value)}
                >
                  <span className="option-marker">{multiSelect ? (active ? "☑" : "☐") : active ? "◉" : "○"}</span>
                  <span className="option-body">
                    <span className="option-label">{o.label}</span>
                    {o.description && <span className="option-desc">{o.description}</span>}
                  </span>
                </button>
              );
            })}
          </div>
          <div className="other-row">
            <input
              className="text-input"
              placeholder="Other…"
              value={other}
              onChange={(e) => setOther(e.target.value)}
            />
            <button className="btn btn-primary" onClick={onSubmit}>
              Submit
            </button>
          </div>
        </>
      )}
    </div>
  );
}
