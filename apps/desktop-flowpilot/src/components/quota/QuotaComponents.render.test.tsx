import test from "node:test";
import assert from "node:assert/strict";
import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { setupDom } from "../../testhelpers/domHarness";
import { QuotaCandidateTable } from "./QuotaCandidateTable";
import { QuotaRoutingAudit } from "./QuotaRoutingAudit";
import { SameProviderCooldownBar } from "./SameProviderCooldownBar";
import type { QuotaRouteDecisionDTO } from "../../types/contract";
import { applyTimelineEvent } from "../../state/timelineReducer";
import type { ProviderEventDTO } from "../../types/contract";

// Task-450 (CP-87 P-6): Desktop quota surfaces — contractual candidate
// columns, runner order verbatim, once/run/stop actions, auto-rotation
// notice, honest unknown headroom, and the server-deadline cooldown bar.

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

function decisionFixture(): QuotaRouteDecisionDTO {
  return {
    runId: "run-1",
    trigger: "quota_exhausted",
    reason: "quota_exhausted",
    providerKey: "codex",
    model: "gpt-5.4",
    accountId: "cx-0",
    policyVersion: 1,
    candidates: [
      {
        providerKey: "codex", model: "gpt-5.4", workloadClass: "coding",
        accountId: "cx-1",
        headroom: { state: "healthy", remainingPercent: 90, freshnessSeconds: 5, confidence: "exact" },
        autoEligible: true,
      },
      {
        providerKey: "claude", model: "claude-sonnet-x", workloadClass: "coding",
        accountId: "cl-1",
        headroom: { state: "unknown", freshnessSeconds: 400, confidence: "none" },
        autoEligible: false, rejectionReasons: ["unknown_quota"],
      },
      {
        providerKey: "codex", model: "gpt-5.4", workloadClass: "coding",
        accountId: "cx-2",
        headroom: { state: "exhausted", remainingPercent: 0, freshnessSeconds: 2, confidence: "exact" },
        autoEligible: false, rejectionReasons: ["exhausted_quota"],
      },
    ],
  };
}

test("candidate table shows provider model workload account headroom reset confidence", async () => {
  const h = await render(<QuotaCandidateTable decision={decisionFixture()} onSelect={() => {}} />);
  try {
    const heads = [...h.container.querySelectorAll("th")].map((th) => th.textContent);
    for (const col of ["Provider", "Model", "Workload", "Account", "Headroom", "Reset", "Confidence", "Reason"]) {
      assert.ok(heads.includes(col), `missing column ${col}: ${heads}`);
    }
    const text = h.container.textContent ?? "";
    assert.match(text, /90%/);
    assert.match(text, /exact/);
  } finally {
    await h.cleanup();
  }
});

test("candidate table preserves runner order and reasons", async () => {
  const h = await render(<QuotaCandidateTable decision={decisionFixture()} onSelect={() => {}} />);
  try {
    const text = h.container.textContent ?? "";
    const i1 = text.indexOf("cx-1");
    const i2 = text.indexOf("cl-1");
    const i3 = text.indexOf("cx-2");
    assert.ok(i1 > 0 && i2 > i1 && i3 > i2, `order lost: ${text}`);
    assert.match(text, /unknown_quota/);
    assert.match(text, /exhausted_quota/);
    // Rejected rows render disabled, never silently dropped.
    const disabled = h.container.querySelectorAll("tr.quota-row-disabled");
    assert.equal(disabled.length, 1);
  } finally {
    await h.cleanup();
  }
});

test("manual selection offers once/run and explicit save-default only when supported", async () => {
  const picks: string[] = [];
  const h = await render(<QuotaCandidateTable decision={decisionFixture()} onSelect={(id) => picks.push(id)} />);
  try {
    const buttons = [...h.container.querySelectorAll("button")];
    const once = buttons.find((b) => b.textContent === "Once");
    const forRun = buttons.find((b) => b.textContent === "For run");
    const stop = buttons.find((b) => b.textContent === "Stop");
    assert.ok(once && forRun && stop, "once/run/stop actions required");
    // Save-as-step-default is deferred per the spec's open question — the
    // action is hidden, not a shipped no-op.
    assert.equal(buttons.some((b) => /save.*default/i.test(b.textContent ?? "")), false);
    await act(async () => {
      once!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    assert.equal(picks[0], "use_once|codex|cx-1|gpt-5.4");
    await act(async () => {
      stop!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    assert.equal(picks[1], "stop");
  } finally {
    await h.cleanup();
  }
});

test("auto rotation notice shows requested and resolved route", () => {
  const ev = {
    id: "e-1", workflowRunId: "run-1", providerSessionId: "s", providerKey: "codex",
    seq: 1, occurredAt: "2026-09-25T00:00:00Z",
    type: "quota_route_committed",
    quotaRoute: { fromProvider: "codex", fromAccount: "cx-0", toProvider: "claude", toAccount: "cl-1", toModel: "claude-sonnet-x" },
  } as ProviderEventDTO;
  const state = applyTimelineEvent({ status: "running", timeline: [], recoverable: false, pendingApprovals: [], pendingQuestions: [] }, ev);
  const notice = (state.timeline ?? []).find((i) => i.kind === "system");
  assert.ok(notice, "committed rotation must leave a timeline notice");
  if (notice?.kind === "system") {
    assert.match(notice.text, /codex\/cx-0 → claude\/cl-1 claude-sonnet-x/);
    assert.equal(notice.tone, "info");
  }
});

test("unknown headroom is not rendered as zero or tokens", async () => {
  const d = decisionFixture();
  const h = await render(<QuotaCandidateTable decision={d} onSelect={() => {}} />);
  try {
    const rows = [...h.container.querySelectorAll("tbody tr")];
    const unknownRow = rows.find((r) => r.textContent?.includes("cl-1"));
    assert.ok(unknownRow);
    assert.match(unknownRow.textContent ?? "", /unknown/);
    assert.doesNotMatch(unknownRow.textContent ?? "", /\d+%.*cl-1|cl-1.*\d+%/);
  } finally {
    await h.cleanup();
  }
});

test("same-provider cooldown bar counts 20 seconds from server timestamps", async () => {
  const now = Date.parse("2026-09-25T12:00:08Z");
  const h = await render(
    <SameProviderCooldownBar
      startedAt="2026-09-25T12:00:00Z"
      until="2026-09-25T12:00:20Z"
      reason="same_provider_ip_safety"
      now={now}
    />,
  );
  try {
    const text = h.container.textContent ?? "";
    assert.match(text, /12s remaining/);
    const bar = h.container.querySelector('[role="progressbar"]');
    assert.ok(bar);
    assert.equal(bar.getAttribute("aria-valuemax"), "20");
    assert.equal(bar.getAttribute("aria-valuenow"), "12");
  } finally {
    await h.cleanup();
  }
});

test("cooldown bar survives remount without restarting at 20 seconds", async () => {
  const startedAt = "2026-09-25T12:00:00Z";
  const until = "2026-09-25T12:00:20Z";
  const first = await render(
    <SameProviderCooldownBar startedAt={startedAt} until={until} reason="same_provider_ip_safety" now={Date.parse("2026-09-25T12:00:18Z")} />,
  );
  try {
    assert.match(first.container.textContent ?? "", /2s remaining/);
  } finally {
    await first.cleanup();
  }
  // Remount at the same server point — the bar resumes, never re-bases.
  const second = await render(
    <SameProviderCooldownBar startedAt={startedAt} until={until} reason="same_provider_ip_safety" now={Date.parse("2026-09-25T12:00:18Z")} />,
  );
  try {
    assert.match(second.container.textContent ?? "", /2s remaining/);
    // Past the deadline the bar shows 0, never negative or extended.
    const third = await render(
      <SameProviderCooldownBar startedAt={startedAt} until={until} reason="same_provider_ip_safety" now={Date.parse("2026-09-25T12:00:25Z")} />,
    );
    try {
      assert.match(third.container.textContent ?? "", /0s remaining/);
    } finally {
      await third.cleanup();
    }
  } finally {
    await second.cleanup();
  }
});

test("cooldown bar explains same-IP provider safety and is not color-only", async () => {
  const h = await render(
    <SameProviderCooldownBar
      startedAt="2026-09-25T12:00:00Z"
      until="2026-09-25T12:00:20Z"
      reason="same_provider_ip_safety"
      now={Date.parse("2026-09-25T12:00:05Z")}
    />,
  );
  try {
    const text = h.container.textContent ?? "";
    assert.match(text, /same provider.*one IP|one IP.*provider/i);
    // State is conveyed as text + ARIA, not color alone.
    const bar = h.container.querySelector('[role="progressbar"]');
    assert.ok(bar?.getAttribute("aria-valuetext"));
    assert.match(text, /15s remaining/);
  } finally {
    await h.cleanup();
  }
});

test("audit drawer correlates requested resolved and usage", async () => {
  const h = await render(
    <QuotaRoutingAudit
      record={{
        runId: "run-1",
        outcome: "committed",
        committedAt: "2026-09-25T12:00:00Z",
        fromProvider: "codex",
        fromAccount: "cx-0",
        toProvider: "codex",
        toAccount: "cx-1",
        toModel: "gpt-5.4",
        scope: "run",
        reason: "quota_exhausted",
        policyVersion: 1,
        headroom: { state: "healthy", remainingPercent: 90, freshnessSeconds: 5, confidence: "exact" },
        estPromptTokens: 2048,
        maxUsageTokens: 600000,
        actualUsage: { cachedInputTokens: 0, inputTokens: 3000, outputTokens: 1321, reasoningOutputTokens: 0, totalTokens: 4321 },
      }}
    />,
  );
  try {
    const text = h.container.textContent ?? "";
    assert.match(text, /committed/);
    assert.match(text, /cx-0/);
    assert.match(text, /cx-1/);
    assert.match(text, /2048/);
    assert.match(text, /4321/);
    assert.match(text, /healthy 90%/);
  } finally {
    await h.cleanup();
  }
});
