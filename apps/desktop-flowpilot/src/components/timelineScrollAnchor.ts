import type { TimelineItem } from "@/state/store";

// Task-433 follow-up (BUG-558): scroll-anchor restore must survive the ways a
// rendered item's id can move between snapshot and restore — a lone
// approval/question re-ids into its group form when a sibling appends, and new
// prompts pushed during the away window can hide the anchored item above the
// visible slice. These helpers keep the resolution logic pure and testable.

/** DOM ids to try for a captured anchor, including regrouped forms. */
export function scrollAnchorCandidates(itemId: string): string[] {
  return [itemId, `approval-group-${itemId}`, `question-group-${itemId}`];
}

/** Number of trailing prompts the visible window must show for the anchored
 *  item to render, or undefined when the item is no longer in the timeline at
 *  all (rebuilt id — the anchor can never resolve and callers should fall
 *  back to the bottom instead of leaving the view clamped at the top). */
export function scrollAnchorRevealPrompts(
  timeline: TimelineItem[],
  itemId: string,
): number | undefined {
  const rawId = itemId.replace(/^(tool|approval|question)-group-/, "");
  const idx = timeline.findIndex((it) => it.id === itemId || it.id === rawId);
  if (idx < 0) return undefined;
  let before = 0;
  for (let i = 0; i <= idx; i++) {
    if (timeline[i].kind === "prompt") before++;
  }
  const total = timeline.reduce((n, it) => n + (it.kind === "prompt" ? 1 : 0), 0);
  return total - before + 1;
}
