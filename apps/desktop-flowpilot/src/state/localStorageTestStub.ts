// Test-only localStorage polyfill. Node has no DOM storage, but the store
// module evaluates loadDrafts()/loadWorkingMode() at import time — import this
// file FIRST in tests that need storage to exist.
const mem = new Map<string, string>();
const g = globalThis as unknown as { localStorage?: Storage };
if (!g.localStorage) {
  g.localStorage = {
    getItem: (k: string) => mem.get(k) ?? null,
    setItem: (k: string, v: string) => void mem.set(k, String(v)),
    removeItem: (k: string) => void mem.delete(k),
    clear: () => mem.clear(),
    key: (i: number) => [...mem.keys()][i] ?? null,
    get length() {
      return mem.size;
    },
  } as Storage;
}
