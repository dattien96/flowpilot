import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import type { ArtifactDefinition } from "@/domain/model/entity/workflow-engine";

interface ArtifactDefinitionSelectorProps {
  label: string;
  description: string;
  selectedArtifactKeys: string[];
  artifactDefinitions: ArtifactDefinition[];
  onChange: (nextArtifactKeys: string[]) => void;
}

export function ArtifactDefinitionSelector({
  label,
  description,
  artifactDefinitions,
  selectedArtifactKeys,
  onChange,
}: ArtifactDefinitionSelectorProps) {
  const firstAvailableArtifactDefinition = (selectedKeys: string[]) =>
    artifactDefinitions.find((option) => !selectedKeys.includes(option.key))?.key ?? "";

  const getArtifactDefinitionOption = (key: string) =>
    artifactDefinitions.find((option) => option.key === key) ?? null;

  const [selectedArtifactKey, setSelectedArtifactKey] = useState(
    firstAvailableArtifactDefinition(selectedArtifactKeys),
  );

  useEffect(() => {
    setSelectedArtifactKey((current) => {
      if (current && !selectedArtifactKeys.includes(current)) {
        return current;
      }
      return firstAvailableArtifactDefinition(selectedArtifactKeys);
    });
  }, [artifactDefinitions, selectedArtifactKeys]);

  const availableArtifactOptions = artifactDefinitions.filter(
    (option) => !selectedArtifactKeys.includes(option.key),
  );

  const addArtifact = () => {
    if (!selectedArtifactKey || selectedArtifactKeys.includes(selectedArtifactKey)) {
      return;
    }

    const nextArtifactKeys = [...selectedArtifactKeys, selectedArtifactKey];
    onChange(nextArtifactKeys);
    setSelectedArtifactKey(firstAvailableArtifactDefinition(nextArtifactKeys));
  };

  const removeArtifact = (artifactKey: string) => {
    const nextArtifactKeys = selectedArtifactKeys.filter((current) => current !== artifactKey);
    onChange(nextArtifactKeys);
    setSelectedArtifactKey(firstAvailableArtifactDefinition(nextArtifactKeys));
  };

  return (
    <div className="space-y-3 rounded-2xl border border-border bg-card p-3">
      <div className="space-y-1">
        <span className="font-medium">{label}</span>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <label className="min-w-[220px] flex-1 space-y-2 text-sm">
          <span className="font-medium">Available artifact</span>
          <select
            className="w-full rounded-2xl border border-border bg-background px-4 py-3"
            value={selectedArtifactKey}
            onChange={(event) => setSelectedArtifactKey(event.target.value)}
          >
            {availableArtifactOptions.length === 0 ? (
              <option value="">No more artifact types available</option>
            ) : null}
            {availableArtifactOptions.map((option) => (
              <option key={option.key} value={option.key}>
                {option.name}
              </option>
            ))}
          </select>
        </label>
        <Button
          disabled={!selectedArtifactKey || availableArtifactOptions.length === 0}
          type="button"
          variant="secondary"
          onClick={addArtifact}
        >
          Add artifact
        </Button>
      </div>
      <div className="flex flex-wrap gap-2">
        {selectedArtifactKeys.length === 0 ? (
          <p className="text-sm text-muted-foreground">No artifacts selected.</p>
        ) : null}
        {selectedArtifactKeys.map((artifactKey) => {
          const option = getArtifactDefinitionOption(artifactKey);

          return (
            <span
              key={artifactKey}
              className="inline-flex items-center gap-2 rounded-full border border-border bg-background px-3 py-1 text-xs font-medium"
            >
              <span>{option?.name ?? artifactKey}</span>
                <span className="text-[10px] uppercase tracking-[0.2em] text-muted-foreground">
                {option?.defaultFileName ?? "custom"}
                </span>
              <button
                className="text-muted-foreground hover:text-foreground"
                type="button"
                onClick={() => removeArtifact(artifactKey)}
              >
                Remove
              </button>
            </span>
          );
        })}
      </div>
    </div>
  );
}
