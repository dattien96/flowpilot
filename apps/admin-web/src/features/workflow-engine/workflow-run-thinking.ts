type WorkflowRunLogLike = {
  createdAt: string;
  message?: string | null;
};

type ProviderStreamPayload = {
  stream?: string;
  text?: string;
};

const SKILL_READ_PATTERN =
  /\b(?:cat|get-content|load(?:ed|ing)?|open(?:ed|ing)?|read(?:ing)?|type)\b[^\n]*?(?:^|[\\/])skills[\\/]+(?:\.system[\\/]+)?([^\\/'"`\s]+)[\\/]+SKILL\.md\b/gi;
const SKILL_ANNOUNCEMENT_PATTERN =
  /\b(?:using|loading|loaded|calling|called|applying|applied)\s+(?:the\s+)?[`"'$]*([a-z0-9][a-z0-9._:-]*)[`"']*\s+skill\b/gi;
const NAMED_SKILL_ACTION_PATTERN =
  /\b(?:using|loading|loaded|calling|called|applying|applied|read(?:ing)?|open(?:ed|ing)?)\s+(?:(?:the|local|project|my|named)\s+){0,4}[`"'$]*([a-z0-9][a-z0-9._:-]*-skill)\b[`"']*(?:\s+instructions?)?/gi;

function collectContentValueTextParts(value: unknown): string[] {
  if (typeof value === "string") {
    const normalized = value.trim();
    return normalized ? [normalized] : [];
  }

  if (Array.isArray(value)) {
    return value.flatMap((entry) => collectContentValueTextParts(entry));
  }

  if (!value || typeof value !== "object") {
    return [];
  }

  const record = value as Record<string, unknown>;
  const parts: string[] = [];

  for (const key of ["text", "content"]) {
    if (!(key in record)) {
      continue;
    }
    parts.push(...collectContentValueTextParts(record[key]));
  }

  return parts;
}

function collectContentTextParts(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.flatMap((entry) => collectContentTextParts(entry));
  }

  if (!value || typeof value !== "object") {
    return [];
  }

  const parts: string[] = [];
  for (const [key, entry] of Object.entries(value as Record<string, unknown>)) {
    if (key === "content") {
      parts.push(...collectContentValueTextParts(entry));
    } else {
      parts.push(...collectContentTextParts(entry));
    }
  }

  return parts;
}

export function extractProviderStreamDisplayText(rawText: string) {
  if (!rawText.trim()) {
    return "";
  }

  const parts: string[] = [];
  let plainTextStart = 0;
  let jsonStart = -1;
  let depth = 0;
  let isInsideString = false;
  let isEscaped = false;

  for (let index = 0; index < rawText.length; index += 1) {
    const character = rawText[index];

    if (jsonStart < 0) {
      if (character === "{" || character === "[") {
        jsonStart = index;
        depth = 1;
      }
      continue;
    }

    if (isInsideString) {
      if (isEscaped) {
        isEscaped = false;
      } else if (character === "\\") {
        isEscaped = true;
      } else if (character === '"') {
        isInsideString = false;
      }
      continue;
    }

    if (character === '"') {
      isInsideString = true;
    } else if (character === "{" || character === "[") {
      depth += 1;
    } else if (character === "}" || character === "]") {
      depth -= 1;
    }

    if (depth !== 0) {
      continue;
    }

    try {
      const parsed = JSON.parse(rawText.slice(jsonStart, index + 1)) as unknown;
      const plainText = rawText.slice(plainTextStart, jsonStart).trim();
      if (plainText) {
        parts.push(plainText);
      }
      parts.push(...collectContentTextParts(parsed));
      plainTextStart = index + 1;
    } catch {
      // Preserve invalid JSON as plain terminal output.
    }

    jsonStart = -1;
    isInsideString = false;
    isEscaped = false;
  }

  if (plainTextStart === 0) {
    return rawText;
  }

  const remainingText = rawText.slice(plainTextStart).trim();
  if (remainingText) {
    parts.push(remainingText);
  }

  return parts.join("\n");
}

export function parseProviderStreamPayload(message: string | null | undefined) {
  const normalized = message?.trim() ?? "";
  if (!normalized.startsWith("provider_stream:")) {
    return null;
  }

  try {
    return JSON.parse(normalized.slice("provider_stream:".length)) as ProviderStreamPayload;
  } catch {
    return null;
  }
}

export function buildThinkingLinesFromLogs(logs: WorkflowRunLogLike[]) {
  const transcript = [...logs]
    .sort((left, right) => left.createdAt.localeCompare(right.createdAt))
    .map((log) => parseProviderStreamPayload(log.message)?.text ?? "")
    .join("")
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n");

  return extractProviderStreamDisplayText(transcript)
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

export function buildLiveThinkingLines(lines: string[], limit = 3) {
  if (limit <= 0) {
    return [];
  }

  return lines.slice(-limit);
}

export function extractSkillAuditEntries(lines: string[]) {
  const skills = new Set<string>();
  const transcript = lines.join("\n");
  const matches = [
    ...Array.from(transcript.matchAll(SKILL_READ_PATTERN)),
    ...Array.from(transcript.matchAll(SKILL_ANNOUNCEMENT_PATTERN)),
    ...Array.from(transcript.matchAll(NAMED_SKILL_ACTION_PATTERN)),
  ].sort((left, right) => (left.index ?? 0) - (right.index ?? 0));

  for (const match of matches) {
    skills.add(match[1]);
  }

  return Array.from(skills);
}

export function isLiveThinkingPrompt({
  promptIndex,
  promptCount,
  sessionStatus,
  stepStatus,
}: {
  promptIndex: number;
  promptCount: number;
  sessionStatus?: string | null;
  stepStatus: string;
}) {
  const isRunningStep = stepStatus.trim().toUpperCase() === "RUNNING";
  const isActiveSession =
    !sessionStatus || sessionStatus.trim().toLowerCase() === "active";

  return isRunningStep && isActiveSession && promptIndex === promptCount - 1;
}

export function findNextPromptCreatedAt(
  promptCreatedAt: string,
  promptCreatedAts: string[],
) {
  return [...promptCreatedAts]
    .sort((left, right) => left.localeCompare(right))
    .find((createdAt) => createdAt > promptCreatedAt) ?? null;
}

export function getPromptWindowThinkingLines({
  logs,
  promptCreatedAt,
  nextPromptCreatedAt,
}: {
  logs: WorkflowRunLogLike[];
  promptCreatedAt: string;
  nextPromptCreatedAt?: string | null;
}) {
  const scopedLogs = logs.filter((log) => {
    if (log.createdAt < promptCreatedAt) {
      return false;
    }

    if (nextPromptCreatedAt && log.createdAt >= nextPromptCreatedAt) {
      return false;
    }

    return parseProviderStreamPayload(log.message) !== null;
  });

  return buildThinkingLinesFromLogs(scopedLogs);
}
