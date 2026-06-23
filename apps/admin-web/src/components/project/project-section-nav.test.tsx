import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { ProjectSectionNav } from "./project-section-nav";

const mocks = vi.hoisted(() => ({
  pathname: "/projects/project-alpha/directory-bindings",
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
  useLocation: () => ({ pathname: mocks.pathname }),
}));

describe("ProjectSectionNav", () => {
  it("includes the directory binding tab", () => {
    render(<ProjectSectionNav projectId="project-alpha" />);

    const tab = screen.getByRole("link", { name: "Directory Binding" });
    expect(tab).toHaveAttribute("href", "/projects/project-alpha/directory-bindings");
  });

  it("includes the engine tab", () => {
    render(<ProjectSectionNav projectId="project-alpha" />);

    const tab = screen.getByRole("link", { name: "Engine" });
    expect(tab).toHaveAttribute("href", "/projects/project-alpha/engine");
  });
});
