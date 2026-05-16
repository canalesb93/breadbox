// The freshly-minted plaintext key only ever exists on the create response;
// the API never returns it again. We stash it in sessionStorage so the
// /api-keys/created subpage can read it on first paint and survive a single
// browser refresh (which is fast enough for users typing into a password
// manager). The store is cleared the moment the user navigates away or
// explicitly dismisses it — see clearPendingPlaintextKey.
const STORAGE_KEY = "breadbox.v2.api-keys.pending-plaintext";

export interface PendingPlaintextKey {
  id: string;
  name: string;
  plaintext: string;
}

export function storePendingPlaintextKey(key: PendingPlaintextKey): void {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(key));
  } catch {
    // SessionStorage can throw in private mode / quota-exceeded — the
    // /created page handles the empty case by redirecting back to the list.
  }
}

export function readPendingPlaintextKey(): PendingPlaintextKey | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as PendingPlaintextKey;
    if (!parsed?.plaintext || !parsed?.name) return null;
    return parsed;
  } catch {
    return null;
  }
}

export function clearPendingPlaintextKey(): void {
  try {
    sessionStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}
