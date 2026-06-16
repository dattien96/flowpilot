import type { PromptAttachment } from "@/types/contract";

// Image attachment normalization (Task-052, D-6). Runs entirely in the renderer
// (Electron/Chromium) using createImageBitmap + OffscreenCanvas, so only normalized
// bytes ever cross the desktop→runner HTTP boundary. Downscaling is the biggest win:
// vision models downscale internally and bill by pixel area, so a full-resolution
// screenshot just wastes bytes and tokens.

/** Long-edge cap: Claude's ~1.15MP sweet spot, also safe for Codex/GPT tiling. */
export const MAX_EDGE = 1568;
/** Preview thumbnail long edge (composer chip + timeline bubble). */
export const THUMB_EDGE = 192;
/** WebP/JPEG quality for recompression. */
export const ENCODE_QUALITY = 0.8;
/** Max attachments per turn. */
export const MAX_ATTACHMENTS = 6;
/** Max encoded size per image AFTER normalization. */
export const MAX_IMAGE_BYTES = 2 * 1024 * 1024;
/** Accepted source MIME types. */
export const ACCEPTED_MIME = ["image/png", "image/jpeg", "image/webp", "image/gif"] as const;

export const ACCEPT_ATTR = ACCEPTED_MIME.join(",");

export interface PendingAttachment extends PromptAttachment {
  /** Renderer-only object/data URL for the preview chip; never sent to the runner. */
  previewUrl: string;
}

export class ImageNormalizeError extends Error {}

function isAcceptedMime(mime: string): boolean {
  return (ACCEPTED_MIME as readonly string[]).includes(mime);
}

function scaledSize(w: number, h: number, maxEdge: number): { w: number; h: number } {
  const scale = Math.min(1, maxEdge / Math.max(w, h)); // downscale only, never upscale
  return { w: Math.max(1, Math.round(w * scale)), h: Math.max(1, Math.round(h * scale)) };
}

async function blobToBase64(blob: Blob): Promise<string> {
  const dataUrl = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error ?? new Error("read failed"));
    reader.onload = () => resolve(String(reader.result));
    reader.readAsDataURL(blob);
  });
  const comma = dataUrl.indexOf(",");
  return comma >= 0 ? dataUrl.slice(comma + 1) : dataUrl;
}

function drawScaled(bmp: ImageBitmap, w: number, h: number): OffscreenCanvas {
  const canvas = new OffscreenCanvas(w, h);
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new ImageNormalizeError("could not get 2d context");
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
export async function normalizeImage(file: File): Promise<PendingAttachment> {
  if (!isAcceptedMime(file.type)) {
    throw new ImageNormalizeError(`Unsupported image type: ${file.type || "unknown"}`);
  }

  const bmp = await createImageBitmap(file);
  try {
    const main = scaledSize(bmp.width, bmp.height, MAX_EDGE);
    const keepPng = file.type === "image/png";
    const mimeType = keepPng ? "image/png" : "image/webp";

    const mainCanvas = drawScaled(bmp, main.w, main.h);
    const blob = await mainCanvas.convertToBlob(
      keepPng ? { type: "image/png" } : { type: "image/webp", quality: ENCODE_QUALITY },
    );
    if (blob.size > MAX_IMAGE_BYTES) {
      throw new ImageNormalizeError(
        `Image too large after compression (${Math.round(blob.size / 1024)} KB). Try a smaller image.`,
      );
    }
    const data = await blobToBase64(blob);

    const thumb = scaledSize(bmp.width, bmp.height, THUMB_EDGE);
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
  } finally {
    bmp.close();
  }
}

/** Strip the renderer-only preview field before sending over the wire. */
export function toWire(att: PendingAttachment): PromptAttachment {
  const { previewUrl: _previewUrl, ...wire } = att;
  return wire;
}
