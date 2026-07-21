/**
 * BUG-291: a regression decision owns a live child gate at the runner. Hiding
 * its modal locally leaves that child Running and the parent cohort waiting
 * with no remaining action surface. A decision-card's secondary action must
 * therefore terminalize the whole flow; plain informational gate blocks may
 * still be acknowledged locally.
 */
export function gateBlockSecondaryAction(options: readonly string[] | undefined): "dismiss" | "stop-flow" {
  return options && options.length > 0 ? "stop-flow" : "dismiss";
}
