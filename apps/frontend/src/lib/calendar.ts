export type CalendarCell = {
  date: Date;
  inCurrentMonth: boolean;
  isToday: boolean;
  key: string;
};

export const WEEKDAY_LABELS_SHORT = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"] as const;

function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

export function buildMonthGrid(viewYear: number, viewMonth: number, today: Date): CalendarCell[] {
  const firstOfMonth = new Date(viewYear, viewMonth, 1);
  const startWeekday = firstOfMonth.getDay();
  const startDate = new Date(firstOfMonth);
  startDate.setDate(1 - startWeekday);

  const todayStart = startOfDay(today);
  const cells: CalendarCell[] = [];
  for (let i = 0; i < 42; i++) {
    const d = new Date(startDate);
    d.setDate(startDate.getDate() + i);
    const inCurrentMonth = d.getMonth() === viewMonth;
    const isToday = startOfDay(d).getTime() === todayStart.getTime();
    cells.push({
      date: d,
      inCurrentMonth,
      isToday,
      key: `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`,
    });
  }
  return cells;
}

export function isoDate(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function parseIsoDate(v: string): Date | null {
  const m = v.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!m) return null;
  const y = Number(m[1]);
  const mo = Number(m[2]);
  const d = Number(m[3]);
  if (mo < 1 || mo > 12 || d < 1 || d > 31) return null;
  const date = new Date(y, mo - 1, d);
  if (date.getFullYear() !== y || date.getMonth() !== mo - 1 || date.getDate() !== d) {
    return null;
  }
  return date;
}

export function formatIsoDatetime(dateIso: string, timeIso: string): string {
  if (!dateIso) return "";
  if (!timeIso) return dateIso;
  return `${dateIso}T${timeIso}`;
}

export function splitIsoDatetime(v: string): { date: string; time: string } {
  if (!v) return { date: "", time: "" };
  const m = v.match(/^(\d{4}-\d{2}-\d{2})(?:T(\d{2}:\d{2})(?::\d{2})?)?$/);
  if (!m) return { date: "", time: "" };
  return { date: m[1] || "", time: m[2] || "" };
}

export function parseIsoTime(v: string): { h: number; m: number } | null {
  const m = v.match(/^(\d{1,2}):(\d{2})$/);
  if (!m) return null;
  const h = Number(m[1]);
  const mi = Number(m[2]);
  if (h < 0 || h > 23 || mi < 0 || mi > 59) return null;
  return { h, m: mi };
}

export function formatIsoTime(h: number, m: number): string {
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
}
