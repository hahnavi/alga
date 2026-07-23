export function formatTime(iso: string): string {
  const d = new Date(iso);
  if (!Number.isFinite(d.getTime()) || d.getUTCFullYear() < 1970) {
    return "\u2014";
  }
  return d.toLocaleString();
}

export function formatDate(iso: string): string {
  const d = new Date(iso);
  if (!Number.isFinite(d.getTime()) || d.getUTCFullYear() < 1970) {
    return "\u2014";
  }
  return d.toLocaleDateString();
}

/** Stable grouping key for date separators (e.g. chat/message day boundaries). Returns `""` for invalid input. */
export function dateSeparatorKey(iso: string): string {
  const d = new Date(iso);
  if (!Number.isFinite(d.getTime())) return "";
  return d.toLocaleDateString();
}

export function formatTimeOnly(iso: string): string {
  const d = new Date(iso);
  if (!Number.isFinite(d.getTime()) || d.getUTCFullYear() < 1970) {
    return "\u2014";
  }
  return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", hour12: false });
}

export function formatTimeAgo(iso: string): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "";
  if (new Date(iso).getUTCFullYear() < 1970) return "\u2014";
  const sec = Math.floor((Date.now() - t) / 1000);
  if (sec < 45) return "just now";
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const h = Math.floor(min / 60);
  if (h < 24) return `${h}h ago`;
  const d = new Date(iso);
  return (
    d.toLocaleDateString(undefined, { month: "short", day: "numeric" }) +
    ", " +
    d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })
  );
}

export function formatDurationFromMs(totalMs: number): string {
  if (totalMs <= 0) return "0m";
  const diffMins = Math.floor(totalMs / 60000);
  if (diffMins < 60) return `${diffMins}m`;
  const diffHours = Math.floor(diffMins / 60);
  const remainMins = diffMins % 60;
  if (diffHours < 24) return `${diffHours}h ${remainMins}m`;
  const diffDays = Math.floor(diffHours / 24);
  return `${diffDays}d ${diffHours % 24}h`;
}

export function formatExpires(t: { expires_at?: string }): string {
  if (!t.expires_at) return "Never";
  return formatTime(t.expires_at);
}

export function formatDateSeparator(iso: string): string {
  const date = new Date(iso);
  if (!Number.isFinite(date.getTime())) return "";
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const msg = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const diff = Math.floor((today.getTime() - msg.getTime()) / 86400000);
  const dateStr = date.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
  if (diff === 0) return `Today, ${dateStr}`;
  if (diff === 1) return `Yesterday, ${dateStr}`;
  if (diff < 7) {
    const weekday = date.toLocaleDateString(undefined, { weekday: "long" });
    return `${weekday}, ${dateStr}`;
  }
  return dateStr;
}

export function localDatetimeToRFC3339(v: string): string | null {
  if (!v.trim()) return null;
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString();
}

// tzLocalToRFC3339 interprets a datetime-local string (e.g. "2026-01-05T09:00")
// as a wall-clock time in the given IANA timezone and returns the absolute
// UTC RFC3339 instant. Use it when a user enters a "wall-clock in tz" that
// must be persisted as a precise instant (e.g. on-call overrides).
export function tzLocalToRFC3339(v: string, timezone: string): string | null {
  if (!v.trim() || !timezone.trim()) return null;
  const m = v.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/);
  if (!m) return null;
  const y = Number(m[1]);
  const mo = Number(m[2]);
  const d = Number(m[3]);
  const h = Number(m[4]);
  const mi = Number(m[5]);
  const s = Number(m[6] || 0);
  const asUtc = Date.UTC(y, mo - 1, d, h, mi, s);
  let tzAsUtc = asUtc;
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: timezone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hour12: false,
    }).formatToParts(new Date(asUtc));
    const get = (t: string) => Number(parts.find((p) => p.type === t)?.value);
    tzAsUtc = Date.UTC(
      get("year"),
      get("month") - 1,
      get("day"),
      get("hour"),
      get("minute"),
      get("second"),
    );
  } catch {
    return null;
  }
  const offsetMs = tzAsUtc - asUtc;
  return new Date(asUtc - offsetMs).toISOString();
}

export function formatTimeFull(iso: string): string {
  const d = new Date(iso);
  if (!Number.isFinite(d.getTime()) || d.getUTCFullYear() < 1970) return "\u2014";
  const datePart = d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
  const timePart = d.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
  const offsetMin = d.getTimezoneOffset();
  const sign = offsetMin <= 0 ? "+" : "-";
  const absH = String(Math.floor(Math.abs(offsetMin) / 60)).padStart(2, "0");
  const absM = String(Math.abs(offsetMin) % 60).padStart(2, "0");
  return `${datePart} ${timePart} (GMT${sign}${absH}:${absM})`;
}
