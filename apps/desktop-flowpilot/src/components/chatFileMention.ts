export type AtFragment = { index: number; query: string };

export function findActiveAt(text: string, cursor: number): AtFragment | null {
  for (let i = cursor - 1; i >= 0; i--) {
    if (text[i] === "@") {
      if (i === 0 || text[i - 1] === " " || text[i - 1] === "\n") {
        return { index: i, query: text.slice(i + 1, cursor) };
      }
      return null;
    }
    if (text[i] === " " || text[i] === "\n") return null;
  }
  return null;
}

export function isAgentAtMention(fragment: AtFragment, agentNames: string[]): boolean {
  if (fragment.index !== 0) return false;
  if (/[./\\]/.test(fragment.query)) return false;
  const name = fragment.query.toLowerCase();
  if (name.length === 0) return false;
  return agentNames.some((agent) => agent.toLowerCase() === name);
}

export function filterWorkspaceFiles(paths: string[], query: string, limit = 40): string[] {
  const q = query.trim().toLowerCase().replace(/\\/g, "/");
  const matched = q.length === 0
    ? paths
    : paths.filter((path) => path.toLowerCase().replace(/\\/g, "/").includes(q));
  return matched.slice(0, limit);
}

export function insertAtMention(
  text: string,
  fragment: AtFragment,
  cursor: number,
  path: string,
): { text: string; cursor: number } {
  const next = text.slice(0, fragment.index) + path + text.slice(cursor);
  return { text: next, cursor: fragment.index + path.length };
}
