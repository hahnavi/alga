export function safeGetItem(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    // intentional: localStorage may throw in private mode or be disabled
    return null;
  }
}

export function safeSetItem(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* ignore storage failures (private mode, quota, disabled storage) */
  }
}
