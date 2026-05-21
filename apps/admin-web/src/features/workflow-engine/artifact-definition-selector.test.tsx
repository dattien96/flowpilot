import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { ArtifactDefinitionSelector } from "./artifact-definition-selector";

const artifactDefinitions = [
  {
    key: "business_idea_artifact",
    name: "Business Idea",
    description: "",
    localPathTemplate: "",
    remotePathTemplate: "",
    defaultFileName: "BusinessIdea.md",
    createdAt: "2026-05-20T00:00:00Z",
    updatedAt: "2026-05-20T00:00:00Z",
  },
  {
    key: "feature_intake_artifact",
    name: "Feature Intake",
    description: "",
    localPathTemplate: "",
    remotePathTemplate: "",
    defaultFileName: "FeatureIntake.md",
    createdAt: "2026-05-20T00:00:00Z",
    updatedAt: "2026-05-20T00:00:00Z",
  },
];

function Harness() {
  const [selectedArtifactKeys, setSelectedArtifactKeys] = useState<string[]>([]);

  return (
    <ArtifactDefinitionSelector
      label="Input artifact definitions"
      description="Select the required inputs."
      artifactDefinitions={artifactDefinitions}
      selectedArtifactKeys={selectedArtifactKeys}
      onChange={setSelectedArtifactKeys}
    />
  );
}

describe("ArtifactDefinitionSelector", () => {
  it("adds and removes artifact definitions", () => {
    render(<Harness />);

    fireEvent.click(screen.getByRole("button", { name: "Add artifact" }));

    expect(screen.getByText("Business Idea")).toBeInTheDocument();
    expect(screen.getByText("BusinessIdea.md")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Remove" }));

    expect(screen.queryByText("BusinessIdea.md")).not.toBeInTheDocument();
  });
});
