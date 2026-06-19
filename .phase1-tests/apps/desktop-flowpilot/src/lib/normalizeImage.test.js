"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const normalizeImage_1 = require("./normalizeImage");
// normalizeImage() itself needs browser APIs (createImageBitmap / OffscreenCanvas)
// and is exercised in the renderer; these cover the DOM-free guarantees.
function pending(overrides = {}) {
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
(0, node_test_1.default)("toWire strips the renderer-only previewUrl so it never crosses the wire", () => {
    const wire = (0, normalizeImage_1.toWire)(pending());
    strict_1.default.equal(wire.previewUrl, undefined);
    // The payload fields the runner needs are preserved.
    strict_1.default.equal(wire.id, "att-1");
    strict_1.default.equal(wire.mimeType, "image/webp");
    strict_1.default.equal(wire.data, "QUJD");
    strict_1.default.equal(wire.kind, "image");
});
(0, node_test_1.default)("limits and accept list are sane", () => {
    strict_1.default.ok(normalizeImage_1.MAX_EDGE >= 1024 && normalizeImage_1.MAX_EDGE <= 2048, "long-edge cap in a vision-friendly range");
    strict_1.default.ok(normalizeImage_1.MAX_ATTACHMENTS >= 1);
    strict_1.default.ok(normalizeImage_1.ACCEPTED_MIME.includes("image/png"));
    strict_1.default.equal(normalizeImage_1.ACCEPT_ATTR, normalizeImage_1.ACCEPTED_MIME.join(","));
});
