import type { PromptAttachment } from "@/types/contract";

// Task-432 (CP-84 P-4): per-chat prompt drafts.
//
// Drafts are device-local intent: they persist to localStorage only, never to
// Drive/Supabase, and survive selectProject/resetRun/app reloads so multi-lane
// triage no longer loses half-typed prompts. The map is LRU-capped so the
// storage footprint stays bounded.
//
// Key derivation (T-1): `chatId` when the chat exists (provider/model switches
// mint new runIds but keep the chat) → else `runId` (workflow runs have no
// chatId) → else `<projectId>:new` for the not-yet-created chat (cleared when
// the first send mints the chat). projectId null → "global:new".

export interface DraftState {
  text: string;
  // Skill mentions picked via the composer picker; @file mentions live inside
  // `text` itself so they persist with it.
  selectedSkills?: string[];
  // Wire-shape attachments (base64 data included) — serializable by contract.
  attachments?: PromptAttachment[];
  updatedAt: number;
}

export const DRAFT_CAP = 50;

const STORAGE_KEY = "fp:promptDrafts";

export function draftKeyFor(
  chatId: string | null | undefined,
  runId: string | null | undefined,
  projectId: string | null | undefined,
): string {
  if (chatId) return chatId;
  if (runId) return runId;
  return `${projectId ?? "global"}:new`;
}

export function isEmptyDraft(draft: DraftState | undefined): boolean {
  if (!draft) return true;
  return (
    draft.text.trim().length === 0 &&
    (draft.attachments?.length ?? 0) === 0 &&
    (draft.selectedSkills?.length ?? 0) === 0
  );
}

export function loadDrafts(): Record<string, DraftState> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as Record<string, DraftState>;
    if (!parsed || typeof parsed !== "object") return {};
    // Drop malformed entries defensively — a corrupt shard must not poison the
    // whole map.
    const out: Record<string, DraftState> = {};
    for (const [key, value] of Object.entries(parsed)) {
      if (value && typeof value === "object" && typeof value.text === "string") {
        out[key] = value;
      }
    }
    return out;
  } catch {
    return {};
  }
}

export function saveDrafts(drafts: Record<string, DraftState>): void {
  try {
    // Prune beyond cap before writing — oldest updatedAt goes first (T-2).
    const entries = Object.entries(drafts);
    let kept = drafts;
    if (entries.length > DRAFT_CAP) {
      const sorted = entries.sort((a, b) => b[1].updatedAt - a[1].updatedAt);
      kept = Object.fromEntries(sorted.slice(0, DRAFT_CAP));
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(kept));
  } catch {
    // Storage full / unavailable — drafts degrade to session-local silently.
  }
}

/** Retain only the keys listed in `keep` (T-4 prune helper). */
export function pruneDrafts(
  drafts: Record<string, DraftState>,
  keep: Set<string>,
): Record<string, DraftState> {
  const out: Record<string, DraftState> = {};
  for (const key of keep) {
    const value = drafts[key];
    if (value) out[key] = value;
  }
  return out;
}
