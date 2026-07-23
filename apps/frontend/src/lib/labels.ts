/**
 * Parse and serialize "key=value per line" label strings. Used by heartbeat
 * and other label-editor forms so they share one parser instead of each
 * page re-implementing the same regex.
 */

export function labelsToText(labels: Record<string, string> | undefined | null): string {
  if (!labels) return "";
  return Object.entries(labels)
    .map(([k, v]) => `${k}=${v}`)
    .join("\n");
}

export function textToLabels(text: string): Record<string, string> {
  const result: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const idx = trimmed.indexOf("=");
    if (idx < 0) continue;
    const key = trimmed.slice(0, idx).trim();
    if (!key) continue;
    result[key] = trimmed.slice(idx + 1).trim();
  }
  return result;
}
