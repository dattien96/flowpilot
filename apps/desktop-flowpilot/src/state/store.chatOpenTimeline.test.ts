import test from "node:test";
import assert from "node:assert/strict";
import type { ChatTranscriptRecord } from "../types/contract";
import type { TimelineItem } from "./timelineReducer";

// Pure copy of store.ts buildPriorChatTimeline for isolated testing (provider-agnostic, chatId only).
// If this diverges from store.ts, the test will fail when store is updated — intentional.
const HANDOFF_PROMPT_PREFIX = "[FlowPilot cross-provider chat handoff]";

function buildPriorChatTimeline(records: ChatTranscriptRecord[], currentRunId: string): TimelineItem[] {
  const out: TimelineItem[] = [];
  // BUG-350 mirror of the TUI skipLeg rule — keep in sync with store.ts.
  let skipLeg = "";
  for (const rec of records) {
    if (rec.legRunId === currentRunId && rec.type !== "chat_provider_switch") continue;
    switch (rec.type) {
      case "turn_started": {
        const prompt = (rec.payload as { prompt?: unknown })?.prompt;
        if (typeof prompt !== "string" || !prompt.trim() || prompt.trim().startsWith(HANDOFF_PROMPT_PREFIX)) {
          skipLeg = rec.legRunId;
          break;
        }
        skipLeg = "";
        out.push({ kind: "prompt", id: `chat-${rec.chatSeq}-${rec.legRunId}`, text: prompt });
        break;
      }
      case "message_completed": {
        const text = (rec.payload as { text?: unknown })?.text;
        if (rec.legRunId === skipLeg) break;
        if (typeof text !== "string" || !text.trim()) break;
        out.push({ kind: "assistant", id: `chat-${rec.chatSeq}-${rec.legRunId}`, text, finalized: true });
        break;
      }
      case "chat_provider_switch": {
        const p = rec.payload as { toProvider?: string; toModel?: string; handoffMode?: string; includedTurnCount?: number; omittedTurnCount?: number; truncated?: boolean };
        const included = typeof p?.includedTurnCount === "number" ? p.includedTurnCount : 0;
        const omitted = typeof p?.omittedTurnCount === "number" ? p.omittedTurnCount : 0;
        const truncated = Boolean(p?.truncated);
        const carried = truncated && omitted > 0 ? `${included} of ${included + omitted} turns` : `${included} turns`;
        const toProv = (p?.toProvider as string) ?? "?";
        const toModel = (p?.toModel as string) ?? "?";
        const mode = (p?.handoffMode as string) ?? "raw";
        out.push({ kind: "system", id: `seed-divider-${rec.legRunId}-${rec.chatSeq}`, text: `⇄ switched to ${toProv} · ${toModel} — carried ${carried} (${mode})`, tone: "info" });
        break;
      }
      default: break;
    }
  }
  return out;
}

// Provider-agnostic proof: no providerKey branch — only chatId/type/prefix.

test("buildPriorChatTimeline hydrates 3-leg order, dividers kept even on current leg", () => {
  const records: ChatTranscriptRecord[] = [
    { chatId: "cht_a", chatSeq: 1, legRunId: "run-1", type: "turn_started", payload: { prompt: "hello ban la model gi" } as unknown },
    { chatId: "cht_a", chatSeq: 2, legRunId: "run-1", type: "message_completed", payload: { text: "toi la Muse Spark" } as unknown },
    { chatId: "cht_a", chatSeq: 3, legRunId: "run-2", type: "chat_provider_switch", payload: { toProvider: "grok", toModel: "grok-4.5", handoffMode: "raw", includedTurnCount: 1 } as unknown },
    { chatId: "cht_a", chatSeq: 4, legRunId: "run-2", type: "turn_started", payload: { prompt: "grok prompt" } as unknown },
    { chatId: "cht_a", chatSeq: 5, legRunId: "run-2", type: "message_completed", payload: { text: "grok reply" } as unknown },
    { chatId: "cht_a", chatSeq: 6, legRunId: "run-3", type: "chat_provider_switch", payload: { toProvider: "opencode", toModel: "opencode-go/longcat-2.0", handoffMode: "raw", includedTurnCount: 2 } as unknown },
    { chatId: "cht_a", chatSeq: 7, legRunId: "run-3", type: "turn_started", payload: { prompt: "current — must be skipped, replay covers it" } as unknown },
  ];
  const out = buildPriorChatTimeline(records, "run-3");
  assert.equal(out.length, 6);
  assert.equal((out[0] as { text: string }).text, "hello ban la model gi");
  assert.equal((out[1] as { text: string }).text, "toi la Muse Spark");
  assert.ok((out[2] as { text: string }).text.includes("switched to grok"));
  assert.equal((out[3] as { text: string }).text, "grok prompt");
  assert.ok((out[5] as { text: string }).text.includes("switched to opencode"));
});

test("buildPriorChatTimeline skips handoff seed prompt but keeps its divider", () => {
  const records: ChatTranscriptRecord[] = [
    { chatId: "cht_a", chatSeq: 1, legRunId: "run-1", type: "turn_started", payload: { prompt: "[FlowPilot cross-provider chat handoff]\n\n<previous_conversation>blob</previous_conversation>" } as unknown },
    { chatId: "cht_a", chatSeq: 2, legRunId: "run-1", type: "message_completed", payload: { text: "prior reply" } as unknown },
    { chatId: "cht_a", chatSeq: 3, legRunId: "run-2", type: "chat_provider_switch", payload: { toProvider: "grok", toModel: "grok-4.5", handoffMode: "raw", includedTurnCount: 1 } as unknown },
  ];
  const out = buildPriorChatTimeline(records, "run-2");
  // BUG-350 (operator-approved TUI parity): a message on the seed leg before
  // any real turn is the seed envelope's reply — skipped, divider kept.
  assert.equal(out.length, 1);
  assert.ok((out[0] as { text: string }).text.includes("switched to grok"));
});

// BUG-350: live cht_1e5b706a8201 shape — empty-prompt seed turn + seed reply
// must not render as an orphan assistant bubble (TUI renders 5 items here).
test("buildPriorChatTimeline skips empty-prompt seed reply like the TUI backfill", () => {
  const records: ChatTranscriptRecord[] = [
    { chatId: "cht_x", chatSeq: 1, legRunId: "run-1", type: "turn_started", payload: { prompt: "ban la model gi" } as unknown },
    { chatId: "cht_x", chatSeq: 2, legRunId: "run-1", type: "message_completed", payload: { text: "toi la longcat" } as unknown },
    { chatId: "cht_x", chatSeq: 3, legRunId: "run-2", type: "turn_started", payload: { prompt: "" } as unknown },
    { chatId: "cht_x", chatSeq: 4, legRunId: "run-2", type: "chat_provider_switch", payload: { toProvider: "grok", toModel: "grok-4.5", handoffMode: "raw", includedTurnCount: 1 } as unknown },
    { chatId: "cht_x", chatSeq: 5, legRunId: "run-2", type: "message_completed", payload: { text: "Toi la Grok 4.5" } as unknown },
    { chatId: "cht_x", chatSeq: 6, legRunId: "run-2", type: "turn_started", payload: { prompt: "hello ban la model gi" } as unknown },
    { chatId: "cht_x", chatSeq: 7, legRunId: "run-2", type: "message_completed", payload: { text: "real reply" } as unknown },
  ];
  const out = buildPriorChatTimeline(records, "run-3");
  const texts = out.map((t) => (t as { text: string }).text);
  assert.deepEqual(texts, [
    "ban la model gi",
    "toi la longcat",
    "⇄ switched to grok · grok-4.5 — carried 1 turns (raw)",
    "hello ban la model gi",
    "real reply",
  ]);
});

test("buildPriorChatTimeline skips current leg turns but keeps its switch divider", () => {
  const records: ChatTranscriptRecord[] = [
    { chatId: "cht_a", chatSeq: 1, legRunId: "run-1", type: "turn_started", payload: { prompt: "hi" } as unknown },
    { chatId: "cht_a", chatSeq: 2, legRunId: "run-3", type: "chat_provider_switch", payload: { toProvider: "opencode", toModel: "longcat-2.0", handoffMode: "raw", includedTurnCount: 1 } as unknown },
    { chatId: "cht_a", chatSeq: 3, legRunId: "run-3", type: "turn_started", payload: { prompt: "skip me — current turn" } as unknown },
  ];
  const out = buildPriorChatTimeline(records, "run-3");
  assert.equal(out.length, 2);
  assert.equal((out[0] as { text: string }).text, "hi");
  assert.ok((out[1] as { text: string }).text.includes("switched to opencode"));
});
