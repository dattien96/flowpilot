package changecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"strings"
)

// ComputeSignature returns h's reproducible intent_signature (SD-21 D-1):
// sha256 over feature_key, the sorted governing doc ids, the sorted
// governing doc content hashes, and the normalized behavior statement.
// Identical inputs always yield the identical hash; code bytes are never
// part of it — only the governing spec + declared behavior.
func ComputeSignature(h CanonicalHead) string {
	ids := append([]string(nil), h.GoverningDocIDs...)
	sort.Strings(ids)

	hashes := make([]string, 0, len(h.GoverningDocHashes))
	for _, v := range h.GoverningDocHashes {
		hashes = append(hashes, v)
	}
	sort.Strings(hashes)

	joined := strings.Join([]string{
		h.FeatureKey,
		strings.Join(ids, ","),
		strings.Join(hashes, ","),
		normalizeBehaviorStatement(h.BehaviorStatement),
	}, "\x1f")

	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}

// normalizeBehaviorStatement makes signature comparison whitespace/case
// insensitive so trivial re-wording of an unchanged statement (or reloading
// it from disk with different line endings) does not spuriously flip the hash.
func normalizeBehaviorStatement(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// HashDoc returns the sha256 hex hash of the file at path. A missing or
// unreadable file returns an error rather than panicking (SD-21 F-1) —
// callers should record the failure (e.g. an empty hash entry) and flag the
// Head, not crash.
func HashDoc(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
