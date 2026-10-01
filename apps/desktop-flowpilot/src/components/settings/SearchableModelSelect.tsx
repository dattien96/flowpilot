import { useEffect, useMemo, useRef, useState } from "react";
import { filterModelOptions, groupModelOptions } from "@/components/settings/stepModelOptions";
import type { StepModelOption } from "@/components/settings/stepModelOptions";

interface SearchableModelSelectProps {
  /** "" represents the empty/inherit choice. */
  value: string;
  options: StepModelOption[];
  onChange: (value: string) => void;
  /** Trigger text when value is empty (e.g. "Select a model..."). */
  placeholder: string;
  /** Optional first option meaning "no override" (e.g. "(inherit)"). */
  emptyLabel?: string;
  disabled?: boolean;
}

// CA-1076: grouped-by-provider + searchable replacement for the flat native
// <select> — with 200+ enabled models a flat list buries entries like
// devin/swe-2-high past the fold. Behaves like a select: emptyLabel is the
// "" option, click picks and closes, Escape/outside-click cancels.
export function SearchableModelSelect({ value, options, onChange, placeholder, emptyLabel, disabled }: SearchableModelSelectProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const rootRef = useRef<HTMLDivElement | null>(null);
  const searchRef = useRef<HTMLInputElement | null>(null);

  const selected = options.find((option) => option.value === value);
  const filtered = useMemo(() => filterModelOptions(options, query), [options, query]);
  const groups = useMemo(() => groupModelOptions(filtered), [filtered]);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    return () => document.removeEventListener("mousedown", onPointerDown);
  }, [open]);

  useEffect(() => {
    if (open) {
      setQuery("");
      searchRef.current?.focus();
    }
  }, [open]);

  const pick = (next: string) => {
    onChange(next);
    setOpen(false);
  };

  const onSearchKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      setOpen(false);
    } else if (event.key === "Enter") {
      event.preventDefault();
      if (filtered.length > 0) pick(filtered[0].value);
    }
  };

  return (
    <div className="model-select" ref={rootRef}>
      <button
        type="button"
        className="model-select-trigger"
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
      >
        <span className={selected ? "model-select-value" : "model-select-value muted"}>
          {selected ? selected.label : value || placeholder}
        </span>
        <span className="model-select-caret" aria-hidden="true">▾</span>
      </button>
      {open && (
        <div className="model-select-popover">
          <input
            ref={searchRef}
            className="model-select-search"
            type="text"
            placeholder="Search models or providers..."
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={onSearchKeyDown}
          />
          <div className="model-select-list">
            {emptyLabel !== undefined && (
              <button
                type="button"
                className={`model-select-option${value === "" ? " selected" : ""}`}
                onClick={() => pick("")}
              >
                {emptyLabel}
              </button>
            )}
            {groups.map((group) => (
              <div className="model-select-group" key={group.providerKey}>
                <div className="model-select-group-label">{group.providerLabel}</div>
                {group.options.map((option) => (
                  <button
                    type="button"
                    key={option.value}
                    className={`model-select-option${option.value === value ? " selected" : ""}`}
                    onClick={() => pick(option.value)}
                  >
                    <span className="model-select-option-label">{option.label}</span>
                    <span className="model-select-option-id">{option.value}</span>
                  </button>
                ))}
              </div>
            ))}
            {filtered.length === 0 && (
              <div className="model-select-empty">No models match “{query}”.</div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
