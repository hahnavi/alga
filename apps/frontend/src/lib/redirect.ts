const SAFE_REDIRECT = /^\/(?!\/)[^\s]*$/;

export function safeRedirectTarget(queryValue: unknown, fallback = "/"): string {
  return isSafeRedirect(queryValue) ? queryValue : fallback;
}

function isSafeRedirect(target: unknown): target is string {
  if (typeof target !== "string") return false;
  if (target.length === 0 || target.length > 512) return false;
  return SAFE_REDIRECT.test(target);
}
