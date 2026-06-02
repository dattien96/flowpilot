import { deflateRawSync } from "node:zlib";
import { describe, expect, it } from "vitest";

import { decodeArtifactSyncBundle } from "./decode-artifact-sync-bundle";

function u16(value: number) {
  const buffer = Buffer.alloc(2);
  buffer.writeUInt16LE(value, 0);
  return buffer;
}

function u32(value: number) {
  const buffer = Buffer.alloc(4);
  buffer.writeUInt32LE(value, 0);
  return buffer;
}

function buildZip(entries: Array<{ path: string; content: string }>) {
  const chunks: Buffer[] = [];
  const centralDirectory: Buffer[] = [];
  let offset = 0;

  for (const entry of entries) {
    const nameBytes = Buffer.from(entry.path, "utf8");
    const rawBytes = Buffer.from(entry.content, "utf8");
    const compressedBytes = deflateRawSync(rawBytes);

    const localHeader = Buffer.concat([
      u32(0x04034b50),
      u16(20),
      u16(0),
      u16(8),
      u16(0),
      u16(0),
      u32(0),
      u32(compressedBytes.length),
      u32(rawBytes.length),
      u16(nameBytes.length),
      u16(0),
      nameBytes,
    ]);

    const centralHeader = Buffer.concat([
      u32(0x02014b50),
      u16(20),
      u16(20),
      u16(0),
      u16(8),
      u16(0),
      u16(0),
      u32(0),
      u32(compressedBytes.length),
      u32(rawBytes.length),
      u16(nameBytes.length),
      u16(0),
      u16(0),
      u16(0),
      u16(0),
      u32(0),
      u32(offset),
      nameBytes,
    ]);

    chunks.push(localHeader, compressedBytes);
    centralDirectory.push(centralHeader);
    offset += localHeader.length + compressedBytes.length;
  }

  const centralDirectoryBytes = Buffer.concat(centralDirectory);
  const eocd = Buffer.concat([
    u32(0x06054b50),
    u16(0),
    u16(0),
    u16(entries.length),
    u16(entries.length),
    u32(centralDirectoryBytes.length),
    u32(offset),
    u16(0),
  ]);

  return Buffer.concat([...chunks, centralDirectoryBytes, eocd]);
}

describe("decodeArtifactSyncBundle", () => {
  it("decodes a zip bundle into relative files", () => {
    const bundle = buildZip([
      { path: "a.json", content: "{\"ok\":true}" },
      { path: "b.txt", content: "beta" },
    ]);

    const files = decodeArtifactSyncBundle(bundle.buffer.slice(bundle.byteOffset, bundle.byteOffset + bundle.byteLength));

    expect(files).toHaveLength(2);
    expect(files[0]?.relativePath).toBe("a.json");
    expect(files[1]?.relativePath).toBe("b.txt");
    expect(files[0]).toBeDefined();
    expect(new TextDecoder().decode(files[0]!.bytes)).toContain("ok");
  });

  it("rejects path traversal entries", () => {
    const bundle = buildZip([{ path: "../escape.txt", content: "nope" }]);

    expect(() =>
      decodeArtifactSyncBundle(
        bundle.buffer.slice(bundle.byteOffset, bundle.byteOffset + bundle.byteLength),
      ),
    ).toThrow(/path traversal/i);
  });
});
