const PREFIX = "anomaly:idempotency:";

const storageKey = (scope: string, fingerprint: string) =>
  `${PREFIX}${scope}:${fingerprint}`;

/**
 * Returns a stable idempotency key for the same logical request, even across
 * reloads. Reusing the key lets the backend return the already-committed
 * result instead of creating a duplicate application/payment.
 */
export function getOrCreateIdempotencyKey(
  scope: string,
  fingerprint: string,
): string {
  const key = storageKey(scope, fingerprint);
  try {
    const existing = window.sessionStorage.getItem(key);
    if (existing) return existing;
    const created = crypto.randomUUID();
    window.sessionStorage.setItem(key, created);
    return created;
  } catch {
    // Storage unavailable (private mode/webview): fall back to a volatile key.
    return crypto.randomUUID();
  }
}

/** Clears a key only after the backend confirmed the write. */
export function clearIdempotencyKey(scope: string, fingerprint: string) {
  try {
    window.sessionStorage.removeItem(storageKey(scope, fingerprint));
  } catch {
    // Ignore storage failures; the key expires with the session anyway.
  }
}
