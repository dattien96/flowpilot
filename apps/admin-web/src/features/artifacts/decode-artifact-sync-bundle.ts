import { inflateRawSync } from "node:zlib";

export interface ArtifactSyncBundleFile {
  relativePath: string;
  bytes: Uint8Array;
}

export interface DecodeArtifactSyncBundleOptions {
  maxEntries?: number;
  maxEntrySize?: number;
  maxTotalSize?: number;
}

const LOCAL_FILE_HEADER_SIGNATURE = 0x04034b50;
const CENTRAL_DIRECTORY_HEADER_SIGNATURE = 0x02014b50;
const END_OF_CENTRAL_DIRECTORY_SIGNATURE = 0x06054b50;

const DEFAULT_MAX_ENTRIES = 1024;
const DEFAULT_MAX_ENTRY_SIZE = 16 * 1024 * 1024;
const DEFAULT_MAX_TOTAL_SIZE = 128 * 1024 * 1024;

function toBytes(bundle: ArrayBuffer | ArrayBufferView) {
  if (bundle instanceof ArrayBuffer) {
    return new Uint8Array(bundle);
  }

  return new Uint8Array(bundle.buffer, bundle.byteOffset, bundle.byteLength);
}

function readUint16(bytes: Uint8Array, offset: number) {
  return bytes[offset] | (bytes[offset + 1] << 8);
}

function readUint32(bytes: Uint8Array, offset: number) {
  return (
    bytes[offset] |
    (bytes[offset + 1] << 8) |
    (bytes[offset + 2] << 16) |
    (bytes[offset + 3] << 24)
  ) >>> 0;
}

function decodeUtf8(bytes: Uint8Array) {
  return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
}

function findEndOfCentralDirectory(bytes: Uint8Array) {
  const minimumSize = 22;
  for (let offset = bytes.length - minimumSize; offset >= 0; offset -= 1) {
    if (readUint32(bytes, offset) === END_OF_CENTRAL_DIRECTORY_SIGNATURE) {
      return offset;
    }
  }

  throw new Error("ZIP end-of-central-directory record not found.");
}

function validateRelativePath(relativePath: string) {
  const trimmed = relativePath.trim();
  if (!trimmed) {
    throw new Error("ZIP entry name is empty.");
  }
  if (trimmed.startsWith("/") || /^[A-Za-z]:/.test(trimmed) || trimmed.startsWith("//")) {
    throw new Error(`ZIP entry uses an absolute path: ${relativePath}`);
  }
  if (trimmed.includes("\\")) {
    throw new Error(`ZIP entry uses a backslash path separator: ${relativePath}`);
  }
  if (trimmed.endsWith("/")) {
    throw new Error(`ZIP entry is a directory and cannot be extracted: ${relativePath}`);
  }

  const segments = trimmed.split("/");
  if (
    segments.some(
      (segment) => segment.length === 0 || segment === "." || segment === "..",
    )
  ) {
    throw new Error(`ZIP entry contains path traversal segments: ${relativePath}`);
  }

  return segments.join("/");
}

function inflateZipEntry(bytes: Uint8Array, compressedOffset: number, compressedSize: number) {
  const compressed = bytes.subarray(compressedOffset, compressedOffset + compressedSize);
  return new Uint8Array(inflateRawSync(Buffer.from(compressed)));
}

function readZipEntry(
  bytes: Uint8Array,
  headerOffset: number,
  compressedSize: number,
  uncompressedSize: number,
) {
  if (readUint32(bytes, headerOffset) !== LOCAL_FILE_HEADER_SIGNATURE) {
    throw new Error("ZIP local file header is malformed.");
  }

  const compressionMethod = readUint16(bytes, headerOffset + 8);
  const fileNameLength = readUint16(bytes, headerOffset + 26);
  const extraFieldLength = readUint16(bytes, headerOffset + 28);
  const dataOffset = headerOffset + 30 + fileNameLength + extraFieldLength;

  if (dataOffset + compressedSize > bytes.length) {
    throw new Error("ZIP entry exceeds the provided buffer.");
  }

  if (compressionMethod === 0) {
    return bytes.subarray(dataOffset, dataOffset + compressedSize);
  }
  if (compressionMethod === 8) {
    const inflated = inflateZipEntry(bytes, dataOffset, compressedSize);
    if (inflated.length !== uncompressedSize) {
      throw new Error("ZIP entry size mismatch after inflation.");
    }
    return inflated;
  }

  throw new Error(`Unsupported ZIP compression method: ${compressionMethod}`);
}

function readCentralDirectoryEntry(bytes: Uint8Array, headerOffset: number) {
  if (readUint32(bytes, headerOffset) !== CENTRAL_DIRECTORY_HEADER_SIGNATURE) {
    throw new Error("ZIP central directory header is malformed.");
  }

  const compressionMethod = readUint16(bytes, headerOffset + 10);
  const compressedSize = readUint32(bytes, headerOffset + 20);
  const uncompressedSize = readUint32(bytes, headerOffset + 24);
  const fileNameLength = readUint16(bytes, headerOffset + 28);
  const extraFieldLength = readUint16(bytes, headerOffset + 30);
  const commentLength = readUint16(bytes, headerOffset + 32);
  const localHeaderOffset = readUint32(bytes, headerOffset + 42);
  const nameOffset = headerOffset + 46;
  const nameBytes = bytes.subarray(nameOffset, nameOffset + fileNameLength);
  const relativePath = validateRelativePath(decodeUtf8(nameBytes));

  return {
    compressionMethod,
    compressedSize,
    uncompressedSize,
    localHeaderOffset,
    nextOffset: nameOffset + fileNameLength + extraFieldLength + commentLength,
    relativePath,
  };
}

export function decodeArtifactSyncBundle(
  bundle: ArrayBuffer | ArrayBufferView,
  options: DecodeArtifactSyncBundleOptions = {},
) {
  const bytes = toBytes(bundle);
  const eocdOffset = findEndOfCentralDirectory(bytes);
  const maxEntries = options.maxEntries ?? DEFAULT_MAX_ENTRIES;
  const maxEntrySize = options.maxEntrySize ?? DEFAULT_MAX_ENTRY_SIZE;
  const maxTotalSize = options.maxTotalSize ?? DEFAULT_MAX_TOTAL_SIZE;
  const centralDirectoryOffset = readUint32(bytes, eocdOffset + 16);
  const centralDirectorySize = readUint32(bytes, eocdOffset + 12);
  const centralDirectoryEnd = centralDirectoryOffset + centralDirectorySize;
  const entries: ArtifactSyncBundleFile[] = [];
  const seenPaths = new Set<string>();
  let totalSize = 0;
  let cursor = centralDirectoryOffset;

  while (cursor < centralDirectoryEnd) {
    const entry = readCentralDirectoryEntry(bytes, cursor);
    if (seenPaths.has(entry.relativePath)) {
      throw new Error(`ZIP bundle contains a duplicate entry: ${entry.relativePath}`);
    }
    if (entry.uncompressedSize > maxEntrySize) {
      throw new Error(`ZIP entry exceeds maximum size: ${entry.relativePath}`);
    }

    const fileBytes = readZipEntry(
      bytes,
      entry.localHeaderOffset,
      entry.compressedSize,
      entry.uncompressedSize,
    );
    if (fileBytes.length > maxEntrySize) {
      throw new Error(`ZIP entry exceeds maximum size: ${entry.relativePath}`);
    }

    totalSize += fileBytes.length;
    if (totalSize > maxTotalSize) {
      throw new Error("ZIP bundle exceeds maximum total size.");
    }

    entries.push({
      relativePath: entry.relativePath,
      bytes: fileBytes,
    });
    seenPaths.add(entry.relativePath);
    cursor = entry.nextOffset;
  }

  if (entries.length > maxEntries) {
    throw new Error("ZIP bundle exceeds maximum entry count.");
  }

  return entries;
}
