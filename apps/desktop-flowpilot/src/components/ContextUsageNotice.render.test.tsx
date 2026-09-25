import test from "node:test";
import assert from "node:assert/strict";
import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { setupDom } from "../testhelpers/domHarness";
import { UsageFigures, ContextNotice } from "./ContextUsageNotice";

// Task-444 (CP-86 P-5) — the est-vs-usage honesty surface. Two labeled
// figures ("prompt ~Nk est" vs "usage Nk"), inline notices for
// aware-pressure and provider compaction, `—` for missing data — never a
// fake zero. Ask-tier/budget decision cards route through the existing
// user_question_required path (covered by QuestionCard tests). Additive-only.

interface Harness {
  container: HTMLElement;
  cleanup: () => Promise<void>;
}

async function render(el: React.ReactElement): Promise<Harness> {
  const restoreDom = setupDom();
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root: Root = createRoot(container);
  await act(async () => {
    root.render(el);
  });
  return {
    container,
    cleanup: async () => {
      await act(async () => {
        root.unmount();
      });
      container.remove();
      restoreDom();
    },
  };
}

// T-1: step card renders prompt ~Nk est and usage Nk as separate labels.
test("usage figures render prompt ~Nk est and usage Nk as separate labels", async () => {
  const h = await render(<UsageFigures estPromptTokens={5200} usageTokens={41800} />);
  try {
    const text = h.container.textContent ?? "";
    assert.match(text, /prompt ~5\.2k est/);
    assert.match(text, /usage 41\.8k/);
  } finally {
    await h.cleanup();
  }
});

// T-4 honesty: missing data renders a dash, never a fake zero.
test("missing usage renders dash not zero", async () => {
  const h = await render(<UsageFigures estPromptTokens={null} usageTokens={null} />);
  try {
    const text = h.container.textContent ?? "";
    assert.match(text, /prompt — est/);
    assert.match(text, /usage —/);
    assert.doesNotMatch(text, /usage 0/);
  } finally {
    await h.cleanup();
  }
});

// T-2: aware pressure renders an inline banner, not a card.
test("aware pressure renders inline banner, not a card", async () => {
  const h = await render(<ContextNotice kind="pressure_aware" ratio={0.83} />);
  try {
    const text = h.container.textContent ?? "";
    assert.match(text, /context at ~83% of window/);
    // Awareness is non-modal: no buttons, no dialog role.
    assert.equal(h.container.querySelectorAll("button").length, 0);
    assert.equal(h.container.querySelector('[role="dialog"]'), null);
  } finally {
    await h.cleanup();
  }
});

// T-3: provider_compacted renders a pinned notice with prev→cur figures.
test("provider_compacted renders pinned notice line", async () => {
  const h = await render(
    <ContextNotice kind="provider_compacted" ratio={0.09} prev={190000} cur={18000} />,
  );
  try {
    const text = h.container.textContent ?? "";
    assert.match(text, /provider compressed context/);
    assert.match(text, /190\.0k→18\.0k/);
    assert.equal(h.container.querySelectorAll("button").length, 0);
  } finally {
    await h.cleanup();
  }
});
