import { useMemo, useState } from "react";
import { useStore } from "@/state/store";

// Chat input with a `/` command + skill picker. Typing "/" opens a filtered
// skill list; picking one attaches it to the next turn (shown as a chip).
export function ChatInput(): React.ReactElement {
  const skills = useStore((s) => s.skills);
  const sendPrompt = useStore((s) => s.sendPrompt);
  const status = useStore((s) => s.status);
  const selectedStepId = useStore((s) => s.selectedStepId);
  const pendingApproval = useStore((s) => s.pendingApproval);
  const pendingQuestion = useStore((s) => s.pendingQuestion);

  const [text, setText] = useState("");
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]);

  const slashQuery = text.startsWith("/") ? text.slice(1).toLowerCase() : null;
  const showPicker = slashQuery !== null;
  const filtered = useMemo(
    () => (slashQuery === null ? [] : skills.filter((s) => s.name.toLowerCase().includes(slashQuery))),
    [skills, slashQuery],
  );

  const blocked = status === "running" || status === "waiting_approval" || status === "waiting_question";
  const canSend = !!selectedStepId && !blocked && text.trim().length > 0 && !showPicker;

  // Add a skill (multi-select). Keeps the picker open so several can be chosen
  // in a row; clearing the text closes it.
  const pickSkill = (name: string) => {
    setSelectedSkills((prev) => (prev.includes(name) ? prev : [...prev, name]));
    setText("");
  };

  const removeSkill = (name: string) => {
    setSelectedSkills((prev) => prev.filter((s) => s !== name));
  };

  const send = () => {
    if (!canSend) return;
    void sendPrompt(text.trim(), selectedSkills);
    setText("");
    setSelectedSkills([]);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !showPicker) {
      e.preventDefault();
      send();
    }
  };

  return (
    <div className="chat-input">
      {showPicker && (
        <div className="skill-picker">
          <div className="skill-picker-head">Skills · pick one or more</div>
          {filtered.length === 0 && <div className="skill-empty">No matching skill</div>}
          {filtered.map((s) => {
            const active = selectedSkills.includes(s.name);
            return (
              <button
                key={s.name}
                className={`skill-item ${active ? "skill-item-active" : ""}`}
                onClick={() => (active ? removeSkill(s.name) : pickSkill(s.name))}
              >
                <span className="skill-mark">{active ? "☑" : "☐"}</span>
                <span className="skill-name">/{s.name}</span>
                {s.description && <span className="skill-desc">{s.description}</span>}
                <span className={`skill-src src-${s.source}`}>{s.source}</span>
              </button>
            );
          })}
        </div>
      )}

      <div className="input-bar">
        {selectedSkills.length > 0 && (
          <div className="skill-chips">
            {selectedSkills.map((name) => (
              <span key={name} className="skill-chip">
                /{name}
                <button className="chip-x" onClick={() => removeSkill(name)} aria-label={`remove ${name}`}>
                  ×
                </button>
              </span>
            ))}
          </div>
        )}
        <textarea
          className="text-area"
          rows={2}
          placeholder={
            blocked
              ? "Waiting for the current turn…"
              : selectedStepId
                ? "Type a message, or / to pick a skill. Enter to send."
                : "Select a step first."
          }
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
          disabled={blocked}
        />
        <button className="btn btn-primary send-btn" onClick={send} disabled={!canSend}>
          Send
        </button>
      </div>

      {(pendingApproval || pendingQuestion) && (
        <div className="input-note">Action required above before continuing.</div>
      )}
    </div>
  );
}
