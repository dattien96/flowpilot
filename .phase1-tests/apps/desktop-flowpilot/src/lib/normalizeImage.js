"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ImageNormalizeError = exports.ACCEPT_ATTR = exports.ACCEPTED_MIME = exports.MAX_IMAGE_BYTES = exports.MAX_ATTACHMENTS = exports.ENCODE_QUALITY = exports.THUMB_EDGE = exports.MAX_EDGE = void 0;
exports.normalizeImage = normalizeImage;
exports.toWire = toWire;
// Image attachment normalization (Task-052, D-6). Runs entirely in the renderer
// (Electron/Chromium) using createImageBitmap + OffscreenCanvas, so only normalized
// bytes ever cross the desktop→runner HTTP boundary. Downscaling is the biggest win:
// vision models downscale internally and bill by pixel area, so a full-resolution
// screenshot just wastes bytes and tokens.
/** Long-edge cap: Claude's ~1.15MP sweet spot, also safe for Codex/GPT tiling. */
exports.MAX_EDGE = 1568;
/** Preview thumbnail long edge (composer chip + timeline bubble). */
exports.THUMB_EDGE = 192;
/** WebP/JPEG quality for recompression. */
exports.ENCODE_QUALITY = 0.8;
/** Max attachments per turn. */
exports.MAX_ATTACHMENTS = 6;
/** Max encoded size per image AFTER normalization. */
exports.MAX_IMAGE_BYTES = 2 * 1024 * 1024;
/** Accepted source MIME types. */
exports.ACCEPTED_MIME = ["image/png", "image/jpeg", "image/webp", "image/gif"];
exports.ACCEPT_ATTR = exports.ACCEPTED_MIME.join(",");
class ImageNormalizeError extends Error {
}
exports.ImageNormalizeError = ImageNormalizeError;
function isAcceptedMime(mime) {
    return exports.ACCEPTED_MIME.includes(mime);
}
function scaledSize(w, h, maxEdge) {
    const scale = Math.min(1, maxEdge / Math.max(w, h)); // downscale only, never upscale
    return { w: Math.max(1, Math.round(w * scale)), h: Math.max(1, Math.round(h * scale)) };
}
async function blobToBase64(blob) {
    const dataUrl = await new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onerror = () => reject(reader.error ?? new Error("read failed"));
        reader.onload = () => resolve(String(reader.result));
        reader.readAsDataURL(blob);
    });
    const comma = dataUrl.indexOf(",");
    return comma >= 0 ? dataUrl.slice(comma + 1) : dataUrl;
}
function drawScaled(bmp, w, h) {
    const canvas = new OffscreenCanvas(w, h);
    const ctx = canvas.getContext("2d");
    if (!ctx)
        throw new ImageNormalizeError("could not get 2d context");
    ctx.drawImage(bmp, 0, 0, w, h);
    return canvas;
}
/**
 * Normalize a picked image file into a wire `PromptAttachment` (base64, downscaled,
 * recompressed) plus a small preview URL. PNG sources are kept as PNG to preserve
 * possible transparency/text crispness; everything else is recompressed to WebP.
 * Throws `ImageNormalizeError` for unsupported types or when the result still exceeds
 * the per-image cap after recompression.
 */
async function normalizeImage(file) {
    if (!isAcceptedMime(file.type)) {
        throw new ImageNormalizeError(`Unsupported image type: ${file.type || "unknown"}`);
    }
    const bmp = await createImageBitmap(file);
    try {
        const main = scaledSize(bmp.width, bmp.height, exports.MAX_EDGE);
        const keepPng = file.type === "image/png";
        const mimeType = keepPng ? "image/png" : "image/webp";
        const mainCanvas = drawScaled(bmp, main.w, main.h);
        const blob = await mainCanvas.convertToBlob(keepPng ? { type: "image/png" } : { type: "image/webp", quality: exports.ENCODE_QUALITY });
        if (blob.size > exports.MAX_IMAGE_BYTES) {
            throw new ImageNormalizeError(`Image too large after compression (${Math.round(blob.size / 1024)} KB). Try a smaller image.`);
        }
        const data = await blobToBase64(blob);
        const thumb = scaledSize(bmp.width, bmp.height, exports.THUMB_EDGE);
        const thumbCanvas = drawScaled(bmp, thumb.w, thumb.h);
        const thumbBlob = await thumbCanvas.convertToBlob({ type: "image/webp", quality: 0.7 });
        const previewUrl = `data:image/webp;base64,${await blobToBase64(thumbBlob)}`;
        return {
            id: crypto.randomUUID(),
            kind: "image",
            originalName: file.name,
            mimeType,
            data,
            sizeBytes: blob.size,
            width: main.w,
            height: main.h,
            previewUrl,
        };
    }
    finally {
        bmp.close();
    }
}
/** Strip the renderer-only preview field before sending over the wire. */
function toWire(att) {
    const { previewUrl: _previewUrl, ...wire } = att;
    return wire;
}
