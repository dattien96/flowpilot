/**
 * BUG-563 (live run-69320): clicking an agent card in the right rail reset
 * the rail's scroll position to the top — the operator lost their place
 * whenever they switched focus (with 4+ running agents the just-clicked row
 * scrolled out of view immediately).
 *
 * The scroll container is `.right-sidebar-stack`, shared by every rail panel
 * (Agents, Accounts, …). React re-renders on a focus switch can transiently
 * shrink the stack's content (a panel unmounts for a frame, or a live list
 * reorders/refills), and the browser clamps scrollTop to the shorter
 * scrollHeight — the clamp is permanent even after the content refills.
 *
 * The fix preserves the user's scroll offset across commits: the component
 * records scrollTop on every scroll event and, after each render, restores it
 * when the browser clamped it below the remembered position. A deliberate
 * user scroll already updated the remembered value through the scroll event,
 * so restoring never fights the user; when the content genuinely became
 * shorter, the write just clamps to the real maximum (identical to the
 * browser's own behaviour).
 */

/** Decide the scrollTop to restore, or null when no restore is needed. */
export function sidebarScrollRestoreTarget(currentTop: number, rememberedTop: number): number | null {
  if (rememberedTop <= 0 || currentTop >= rememberedTop) return null;
  return rememberedTop;
}
