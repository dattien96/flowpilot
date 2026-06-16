import test from "node:test";
import assert from "node:assert/strict";
import {
  ACCEPTED_MIME,
  ACCEPT_ATTR,
  MAX_ATTACHMENTS,
  MAX_EDGE,
  toWire,
  type PendingAttachment,
} from "./normalizeImage";

// normalizeImage() itself needs browser APIs (createImageBitmap / OffscreenCanvas)
// and is exercised in the renderer; these cover the DOM-free guarantees.

function pending(overrides: Partial<PendingAttachment> = {}): PendingAttachment {
  return {
    id: "att-1",
    kind: "image",
    originalName: "shot.webp",
    mimeType: "image/webp",
    data: "QUJD",
    sizeBytes: 3,
    width: 800,
    height: 600,
    previewUrl: "data:image/webp;base64,QUJD",
    ...overrides,
  };
}

test("toWire strips the renderer-only previewUrl so it never crosses the wire", () => {
  const wire = toWire(pending());
  assert.equal((wire as unknown as Record<string, unknown>).previewUrl, undefined);
  // The payload fields the runner needs are preserved.
  assert.equal(wire.id, "att-1");
  assert.equal(wire.mimeType, "image/webp");
  assert.equal(wire.data, "QUJD");
  assert.equal(wire.kind, "image");
});

test("limits and accept list are sane", () => {
  assert.ok(MAX_EDGE >= 1024 && MAX_EDGE <= 2048, "long-edge cap in a vision-friendly range");
  assert.ok(MAX_ATTACHMENTS >= 1);
  assert.ok(ACCEPTED_MIME.includes("image/png"));
  assert.equal(ACCEPT_ATTR, ACCEPTED_MIME.join(","));
});
