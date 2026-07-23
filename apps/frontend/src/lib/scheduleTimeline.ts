import type { ScheduleShift } from "@/lib/api";

export type TimelineRange = "1d" | "1w" | "2w" | "1m";

export const RANGE_DAYS: Record<TimelineRange, number> = {
  "1d": 1,
  "1w": 7,
  "2w": 14,
  "1m": 30,
};

export const RANGE_LABEL: Record<TimelineRange, string> = {
  "1d": "1 day",
  "1w": "1 week",
  "2w": "2 weeks",
  "1m": "1 month",
};

export const MONTH_NAMES = [
  "January",
  "February",
  "March",
  "April",
  "May",
  "June",
  "July",
  "August",
  "September",
  "October",
  "November",
  "December",
];

export const WEEK_DAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

export function startOfDay(d: Date): Date {
  const date = new Date(d);
  date.setHours(0, 0, 0, 0);
  return date;
}

export function isToday(d: Date): boolean {
  const now = new Date();
  return (
    d.getDate() === now.getDate() &&
    d.getMonth() === now.getMonth() &&
    d.getFullYear() === now.getFullYear()
  );
}

export function isoWeekNumber(d: Date): number {
  const date = new Date(d);
  date.setHours(0, 0, 0, 0);
  date.setDate(date.getDate() + 3 - ((date.getDay() + 6) % 7));
  const week1 = new Date(date.getFullYear(), 0, 4);
  return (
    1 +
    Math.round(((date.getTime() - week1.getTime()) / 86400000 - 3 + ((week1.getDay() + 6) % 7)) / 7)
  );
}

/** Compute the `from`/`to` ISO bounds for a given cursor + range + view. */
export function computeRangeBounds(
  cursor: Date,
  view: "timeline" | "calendar",
  range: TimelineRange,
): { from: string; to: string } {
  const from = startOfDay(cursor);
  const to = new Date(from);
  if (view === "calendar") {
    from.setDate(1);
    to.setMonth(to.getMonth() + 1);
  } else {
    to.setDate(to.getDate() + RANGE_DAYS[range]);
  }
  return { from: from.toISOString(), to: to.toISOString() };
}

/** Inline style that places a shift block on a timeline. */
export function shiftStyle(
  shift: ScheduleShift,
  rangeStartMs: number,
  rangeMs: number,
): Record<string, string> {
  const start = new Date(shift.start).getTime();
  const end = new Date(shift.end).getTime();
  const leftPct = Math.max(0, ((start - rangeStartMs) / rangeMs) * 100);
  const rightEdge = Math.min(100, ((end - rangeStartMs) / rangeMs) * 100);
  const widthPct = Math.max(2, rightEdge - leftPct);
  return { left: `${leftPct}%`, width: `${widthPct}%` };
}

/** True if the shift overlaps the visible range. */
export function shiftWithinRange(
  shift: ScheduleShift,
  rangeStartMs: number,
  rangeEndMs: number,
): boolean {
  const start = new Date(shift.start).getTime();
  const end = new Date(shift.end).getTime();
  return end > rangeStartMs && start < rangeEndMs;
}

export type TimeTick = { label: string; pct: number; isToday: boolean };

/** Build the time scale ticks for the timeline header. */
export function buildTimeScale(
  rangeStart: Date,
  range: TimelineRange,
  totalDays: number,
): TimeTick[] {
  if (range === "1d") {
    const ticks: TimeTick[] = [];
    for (let i = 0; i <= 24; i += 4) {
      ticks.push({
        label: `${String(i).padStart(2, "0")}:00`,
        pct: (i / 24) * 100,
        isToday: true,
      });
    }
    return ticks;
  }
  const ticks: TimeTick[] = [];
  for (let i = 0; i <= totalDays; i++) {
    const d = new Date(rangeStart);
    d.setDate(d.getDate() + i);
    const isFirstOfWeek = i === 0 || d.getDay() === 1;
    const showLabel = totalDays <= 7 || isFirstOfWeek || i === totalDays;
    const weekday = WEEK_DAYS[(d.getDay() + 6) % 7];
    ticks.push({
      label: showLabel ? `${weekday}, ${MONTH_NAMES[d.getMonth()].slice(0, 3)} ${d.getDate()}` : "",
      pct: (i / totalDays) * 100,
      isToday: isToday(d),
    });
  }
  return ticks;
}

export type CalendarCell = {
  date: Date;
  inMonth: boolean;
  isToday: boolean;
  isWeekend: boolean;
  weekNumber: number;
  shifts: ScheduleShift[];
};

/** Build a 6-week calendar grid (42 cells) for the cursor's month. */
export function buildCalendarDays(cursor: Date, shifts: ScheduleShift[]): CalendarCell[] {
  const year = cursor.getFullYear();
  const month = cursor.getMonth();
  const first = new Date(year, month, 1);
  const lead = (first.getDay() + 6) % 7;
  const start = new Date(year, month, 1 - lead);
  const cells: CalendarCell[] = [];
  for (let i = 0; i < 42; i++) {
    const d = new Date(start);
    d.setDate(start.getDate() + i);
    const dayStart = new Date(d);
    dayStart.setHours(0, 0, 0, 0);
    const dayEnd = new Date(dayStart);
    dayEnd.setDate(dayEnd.getDate() + 1);
    const dayShifts = shifts.filter((s) => {
      const ss = new Date(s.start).getTime();
      const se = new Date(s.end).getTime();
      return se > dayStart.getTime() && ss < dayEnd.getTime();
    });
    cells.push({
      date: d,
      inMonth: d.getMonth() === month,
      isToday: isToday(d),
      isWeekend: d.getDay() === 0 || d.getDay() === 6,
      weekNumber: isoWeekNumber(d),
      shifts: dayShifts,
    });
  }
  return cells;
}

/** Get unique user IDs in shift order. */
export function getParticipants(items: ScheduleShift[]): string[] {
  const ids: string[] = [];
  for (const s of items) {
    if (!ids.includes(s.user_id)) ids.push(s.user_id);
  }
  return ids;
}

/** Format a date range for the timeline range label. */
export function formatRangeLabel(rangeStart: Date, range: TimelineRange): string {
  const fmt = (d: Date) => `${MONTH_NAMES[d.getMonth()].slice(0, 3)} ${d.getDate()}`;
  if (range === "1d") return fmt(rangeStart);
  const end = new Date(rangeStart);
  end.setDate(end.getDate() + RANGE_DAYS[range] - 1);
  return `${fmt(rangeStart)} – ${fmt(end)}`;
}
