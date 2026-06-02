import {
  buildLiveThinkingLines,
  buildThinkingLinesFromLogs,
  extractSkillAuditEntries,
  findNextPromptCreatedAt,
  getPromptWindowThinkingLines,
  isLiveThinkingPrompt,
  parseProviderStreamPayload,
} from "./workflow-run-thinking";

describe("workflow-run-thinking", () => {
  it("parses provider stream payloads", () => {
    expect(
      parseProviderStreamPayload(
        'provider_stream:{"stream":"stdout","text":"thinking line"}',
      ),
    ).toEqual({
      stream: "stdout",
      text: "thinking line",
    });
  });

  it("builds thinking lines from streamed logs", () => {
    const lines = buildThinkingLinesFromLogs([
      {
        createdAt: "2026-06-02T05:00:00.000Z",
        message: 'provider_stream:{"stream":"stdout","text":"line 1\\nline"}',
      },
      {
        createdAt: "2026-06-02T05:00:01.000Z",
        message: 'provider_stream:{"stream":"stdout","text":" 2\\n\\nline 3"}',
      },
    ]);

    expect(lines).toEqual(["line 1", "line 2", "line 3"]);
  });

  it("extracts human-readable content from streamed json chunks", () => {
    const lines = buildThinkingLinesFromLogs([
      {
        createdAt: "2026-06-02T05:00:00.000Z",
        message:
          'provider_stream:{"stream":"stdout","text":"{\\"message\\":{\\"content\\":[{\\"type\\":\\"text\\",\\"text\\":\\"thinking item\\"}]}}"}',
      },
    ]);

    expect(lines).toEqual(["thinking item"]);
  });

  it("extracts content from concatenated protocol envelopes", () => {
    const lines = buildThinkingLinesFromLogs([
      {
        createdAt: "2026-06-02T05:00:00.000Z",
        message:
          'provider_stream:{"stream":"stdout","text":"{\\"method\\":\\"codex/event\\",\\"params\\":{\\"msg\\":{\\"type\\":\\"mcp_startup_update\\"}}}{\\"method\\":\\"codex/event\\",\\"params\\":{\\"msg\\":{\\"item\\":{\\"content\\":[{\\"type\\":\\"text\\",\\"text\\":\\"visible thinking\\"}]}}}}"}',
      },
    ]);

    expect(lines).toEqual(["visible thinking"]);
  });

  it("extracts content after protocol envelopes are joined across logs", () => {
    const lines = buildThinkingLinesFromLogs([
      {
        createdAt: "2026-06-02T05:00:00.000Z",
        message:
          'provider_stream:{"stream":"stdout","text":"{\\"params\\":{\\"msg\\":{\\"item\\":{\\"content\\":{\\"te"}',
      },
      {
        createdAt: "2026-06-02T05:00:01.000Z",
        message:
          'provider_stream:{"stream":"stdout","text":"xt\\":\\"joined content\\"}}}}}"}',
      },
    ]);

    expect(lines).toEqual(["joined content"]);
  });

  it("keeps partial live lines before a newline arrives", () => {
    const lines = buildThinkingLinesFromLogs([
      {
        createdAt: "2026-06-02T05:00:00.000Z",
        message: 'provider_stream:{"stream":"stdout","text":"partial line"}',
      },
    ]);

    expect(lines).toEqual(["partial line"]);
  });

  it("keeps only the latest three live thinking lines", () => {
    expect(
      buildLiveThinkingLines(["1", "2", "3", "4"]),
    ).toEqual(["2", "3", "4"]);
  });

  it("extracts unique skill audit entries from paths and announcements", () => {
    expect(
      extractSkillAuditEntries([
        "Loading .agents/skills/coding/SKILL.md",
        "Using the `code-style` skill for the implementation.",
        "Loading C:\\Users\\dev\\.codex\\skills\\.system\\imagegen\\SKILL.md",
        "Loading .agents/skills/coding/SKILL.md again",
      ]),
    ).toEqual(["coding", "code-style", "imagegen"]);
  });

  it("ignores relative skill links mentioned inside loaded skill content", () => {
    expect(
      extractSkillAuditEntries([
        "See [`compose-ui`](../compose-ui/SKILL.md) when developing Compose UI.",
      ]),
    ).toEqual([]);
  });

  it("ignores available skill catalogs that were not used by the prompt", () => {
    expect(
      extractSkillAuditEntries([
        "- imagegen: Generate raster assets. (file: C:\\Users\\dev\\.codex\\skills\\.system\\imagegen\\SKILL.md)",
        "- coding-skill: Enforce implementation standards. (file: C:\\working\\flowpilot\\.agents\\skills\\coding\\SKILL.md)",
      ]),
    ).toEqual([]);
  });

  it("extracts named skill actions when the identifier already ends with skill", () => {
    expect(
      extractSkillAuditEntries([
        "I’m using the `git-commit-skill` because you named it directly.",
        "Read `git-commit-skill`.",
        "I’m opening the local `git-commit-skill` instructions and will summarize what they require.",
      ]),
    ).toEqual(["git-commit-skill"]);
  });

  it("treats the latest prompt in an active running session as live", () => {
    expect(
      isLiveThinkingPrompt({
        promptIndex: 1,
        promptCount: 2,
        sessionStatus: "active",
        stepStatus: "running",
      }),
    ).toBe(true);
    expect(
      isLiveThinkingPrompt({
        promptIndex: 0,
        promptCount: 2,
        sessionStatus: "active",
        stepStatus: "RUNNING",
      }),
    ).toBe(false);
  });

  it("finds the next prompt across session groups", () => {
    expect(
      findNextPromptCreatedAt("2026-06-02T05:00:01.000Z", [
        "2026-06-02T05:00:04.000Z",
        "2026-06-02T05:00:01.000Z",
        "2026-06-02T05:00:08.000Z",
      ]),
    ).toBe("2026-06-02T05:00:04.000Z");
  });

  it("limits thinking lines to a prompt window", () => {
    const lines = getPromptWindowThinkingLines({
      logs: [
        {
          createdAt: "2026-06-02T05:00:00.000Z",
          message: 'provider_stream:{"stream":"stdout","text":"before\\n"}',
        },
        {
          createdAt: "2026-06-02T05:00:02.000Z",
          message: 'provider_stream:{"stream":"stdout","text":"one\\n"}',
        },
        {
          createdAt: "2026-06-02T05:00:03.000Z",
          message: 'provider_stream:{"stream":"stdout","text":"two\\n"}',
        },
        {
          createdAt: "2026-06-02T05:00:04.000Z",
          message: 'provider_stream:{"stream":"stdout","text":"next prompt\\n"}',
        },
      ],
      promptCreatedAt: "2026-06-02T05:00:01.000Z",
      nextPromptCreatedAt: "2026-06-02T05:00:04.000Z",
    });

    expect(lines).toEqual(["one", "two"]);
  });
});
