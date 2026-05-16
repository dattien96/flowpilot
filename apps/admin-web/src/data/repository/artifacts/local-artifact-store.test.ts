import { describe, expect, it } from "vitest";

import { sanitize } from "./local-artifact-store";

describe("sanitize", () => {
  it("normalizes path separators and unsafe characters for artifact paths", () => {
    expect(sanitize("../project:alpha/mobile app")).toBe("mobile_app");
    expect(sanitize("..\\project:alpha\\feature one")).toBe("feature_one");
  });

  it("falls back for empty or dot-only values", () => {
    expect(sanitize("")).toBe("unassigned");
    expect(sanitize("..")).toBe("unassigned");
  });
});
