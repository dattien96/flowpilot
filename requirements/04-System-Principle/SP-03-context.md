# 1. Context must be explicit

AI should not guess hidden context. Each workflow run must show:
- what context was used,
- what files were attached,
- what skills were loaded,
- what model was used,
- what prompt template/version was used,
- what output was generated.

# 2. Context can come from multiple sources:
- MCP Servers (Jira, Git, Figma, database, API, etc.),
- uploaded files (PDF, DOCX, Markdown, images, audio, etc.),
- project memory / conversation history,
- system-provided context (time, date, user timezone, project settings).

For each source, we should show:
- what documents/items were selected,
- how they were converted into model-usable context,
- how much token budget they consumed.