export type TaskBoardStatus = "backlog" | "ready" | "in_progress" | "blocked" | "done";

export interface ParsedTaskBreakdownTask {
  id: string;
  title: string;
  rawText: string;
  milestone: string | null;
  status: TaskBoardStatus;
  assignee: string | null;
  estimateHours: number | null;
  dependencies: string[];
}

export interface ParsedTaskBreakdownSection {
  title: string;
  tasks: ParsedTaskBreakdownTask[];
}

export interface ParsedTaskBreakdown {
  title: string;
  summary: string | null;
  sections: ParsedTaskBreakdownSection[];
  tasks: ParsedTaskBreakdownTask[];
  rawMarkdown: string;
}

const headingPattern = /^(#{1,3})\s+(.+)$/;
const listItemPattern = /^(?:[-*+]|\d+\.)\s+(.*)$/;
const estimatePattern = /\b(?:estimate|effort|hours?)\s*[:=]\s*([0-9]+(?:\.[0-9]+)?)\s*h?\b/i;
const assigneePattern = /\b(?:assignee|owner|assigned to|review owner)\s*[:=]\s*([^|,;]+)/i;
const dependencyPattern = /\b(?:depends on|depends|blocks?|blocked by)\s*[:=]\s*([^|]+)/i;

function normalizeText(value: string) {
  return value.trim().replace(/\s+/g, " ");
}

function normalizeStatus(value: string): TaskBoardStatus {
  const text = value.toLowerCase();

  if (/\[(x|done|complete|completed)\]/i.test(text) || /\b(done|completed|finished)\b/i.test(text)) {
    return "done";
  }

  if (/\bblocked\b/i.test(text)) {
    return "blocked";
  }

  if (/\b(in progress|in-progress|doing|wip|working)\b/i.test(text)) {
    return "in_progress";
  }

  if (/\bbacklog\b/i.test(text)) {
    return "backlog";
  }

  if (/^\[ \]/i.test(text)) {
    return "backlog";
  }

  return "ready";
}

function stripListMarkers(value: string) {
  return value
    .replace(/^(?:[-*+]|\d+\.)\s+/, "")
    .replace(/^\[(?: |x|X|done|complete|completed)\]\s*/, "")
    .trim();
}

function stripMetadataFragments(value: string) {
  return value
    .replace(/\s*\|\s*(?:assignee|owner|assigned to|review owner|estimate|effort|hours?|depends on|depends|blocks?|blocked by)\s*[:=][^|]+/gi, "")
    .replace(/\s+(?:assignee|owner|assigned to|review owner|estimate|effort|hours?|depends on|depends|blocks?|blocked by)\s*[:=][^|,;]+/gi, "")
    .replace(/\s*\((?:in progress|blocked|done|completed|backlog|wip|working)[^)]*\)/gi, "")
    .replace(/\s+\((?:assignee|owner|estimate|effort|hours?|blocked|in progress|done|completed|backlog)[^)]*\)\s*$/gi, "")
    .trim();
}

function parseDependencies(value: string) {
  const match = value.match(dependencyPattern);
  if (!match) {
    return [];
  }

  return match[1]
    .split(/[,;/]| and /i)
    .map((item) => normalizeText(item))
    .filter((item) => item.length > 0);
}

function parseEstimateHours(value: string) {
  const match = value.match(estimatePattern);
  if (!match) {
    return null;
  }

  const parsed = Number.parseFloat(match[1]);
  return Number.isFinite(parsed) ? parsed : null;
}

function parseAssignee(value: string) {
  const match = value.match(assigneePattern);
  if (!match) {
    return null;
  }

  const normalized = normalizeText(match[1]).replace(/\s*\)\s*$/, "");
  return normalized.length > 0 ? normalized : null;
}

function parseTaskLine(line: string, index: number, currentMilestone: string | null): ParsedTaskBreakdownTask {
  const status = normalizeStatus(line);
  const listBody = stripListMarkers(line);
  const title = normalizeText(stripMetadataFragments(listBody)) || `Task ${index + 1}`;

  return {
    id: `task-${index + 1}`,
    title,
    rawText: listBody,
    milestone: currentMilestone,
    status,
    assignee: parseAssignee(listBody),
    estimateHours: parseEstimateHours(listBody),
    dependencies: parseDependencies(listBody),
  };
}

function createFallbackTask(markdown: string): ParsedTaskBreakdownTask {
  const excerpt = normalizeText(markdown).slice(0, 220);

  return {
    id: "task-1",
    title: excerpt || "No task items detected",
    rawText: excerpt || "No task items detected",
    milestone: null,
    status: "backlog",
    assignee: null,
    estimateHours: null,
    dependencies: [],
  };
}

export function parseTaskBreakdownMarkdown(markdown: string): ParsedTaskBreakdown {
  const lines = markdown.replaceAll("\r\n", "\n").split("\n");
  const tasks: ParsedTaskBreakdownTask[] = [];
  const sections: ParsedTaskBreakdownSection[] = [];
  const headingSections = new Map<string, ParsedTaskBreakdownSection>();

  let currentMilestone: string | null = null;
  let title = "Task Breakdown";
  let summaryLines: string[] = [];
  let sawContent = false;
  let taskIndex = 0;

  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (!line) {
      continue;
    }

    const headingMatch = line.match(headingPattern);
    if (headingMatch) {
      const headingTitle = normalizeText(headingMatch[2]);
      if (headingMatch[1].length === 1 && title === "Task Breakdown") {
        title = headingTitle;
      }

      currentMilestone = headingTitle;
      if (!headingSections.has(headingTitle)) {
        const section = { title: headingTitle, tasks: [] as ParsedTaskBreakdownTask[] };
        headingSections.set(headingTitle, section);
        sections.push(section);
      }
      sawContent = true;
      continue;
    }

    const taskMatch = line.match(listItemPattern);
    if (taskMatch) {
      const task = parseTaskLine(taskMatch[1], taskIndex, currentMilestone);
      taskIndex += 1;
      tasks.push(task);
      sawContent = true;

      const sectionKey = currentMilestone ?? "General";
      if (!headingSections.has(sectionKey)) {
        const section = { title: sectionKey, tasks: [] as ParsedTaskBreakdownTask[] };
        headingSections.set(sectionKey, section);
        sections.push(section);
      }

      headingSections.get(sectionKey)?.tasks.push(task);
      continue;
    }

    if (!sawContent) {
      summaryLines.push(line);
      if (summaryLines.length > 3) {
        summaryLines = summaryLines.slice(-3);
      }
    }
  }

  if (tasks.length === 0) {
    const fallbackTask = createFallbackTask(markdown);
    tasks.push(fallbackTask);
    sections.push({
      title: currentMilestone ?? "General",
      tasks: [fallbackTask],
    });
  }

  const summary = summaryLines.length > 0 ? summaryLines.join(" ") : null;
  if (summary && title === "Task Breakdown") {
    title = summary.slice(0, 80);
  }

  return {
    title,
    summary,
    sections,
    tasks,
    rawMarkdown: markdown,
  };
}

export function groupTaskBreakdownTasksByStatus(tasks: ParsedTaskBreakdownTask[]) {
  const groups: Record<TaskBoardStatus, ParsedTaskBreakdownTask[]> = {
    backlog: [],
    ready: [],
    in_progress: [],
    blocked: [],
    done: [],
  };

  for (const task of tasks) {
    groups[task.status].push(task);
  }

  return groups;
}

export function formatTaskBoardStatus(status: TaskBoardStatus) {
  switch (status) {
    case "backlog":
      return "Backlog";
    case "ready":
      return "Ready";
    case "in_progress":
      return "In Progress";
    case "blocked":
      return "Blocked";
    case "done":
      return "Done";
  }
}
