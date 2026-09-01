const FOCUS_RING =
  "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]";
const ICON_BTN_BASE =
  "flex cursor-pointer items-center justify-center rounded-md text-[var(--text-muted)] transition-colors";
const MENU_ITEM_BASE =
  "flex w-full cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors";

export const HEADER_ICON_BTN_CLASS = `${ICON_BTN_BASE} h-9 w-9 shrink-0 hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] ${FOCUS_RING}`;

export const ROW_ICON_BTN_CLASS = `${ICON_BTN_BASE} h-9 w-9 shrink-0 hover:bg-[var(--btn-default-hover)] hover:text-[var(--text-primary)] ${FOCUS_RING}`;

export const CARD_ICON_BTN_CLASS = `${ICON_BTN_BASE} h-8 w-8 shrink-0 hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] ${FOCUS_RING}`;

export const POPOVER_MENU_PANEL_CLASS =
  "absolute right-0 top-full z-20 mt-1 min-w-[11rem] rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-1 shadow-lg ring-1 ring-black/5 dark:ring-white/10";

export const POPOVER_MENU_ITEM_CLASS = `${MENU_ITEM_BASE} text-[var(--text-primary)] hover:bg-[var(--btn-default-hover)] disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent`;

export const POPOVER_MENU_DESTRUCTIVE_CLASS = `${MENU_ITEM_BASE} text-[var(--text-error)] hover:bg-red-500/10 hover:text-red-600 dark:hover:text-red-400 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent`;

export const POPOVER_MENU_ITEM_ICON_CLASS = "h-4 w-4 shrink-0";

export const POPOVER_MENU_SEPARATOR_CLASS = "my-1 h-px bg-[var(--border-primary)]";

export const ACCOUNT_MENU_ITEM_CLASS = `flex min-h-11 w-full cursor-pointer items-center gap-2 rounded-md px-3 py-2.5 text-left text-sm transition-colors hover:bg-[var(--sidebar-hover,rgb(148_163_184/0.12))] disabled:cursor-not-allowed disabled:opacity-50 ${FOCUS_RING}`;

export const MOBILE_MORE_USER_ACTION_CLASS =
  "flex w-full cursor-pointer items-center gap-3 rounded-lg px-3 py-2.5 text-sm text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]";

// Filled (solid-background) badge classes for incident priority / severity /
// impact chips. Detail pages render the literal value (e.g. "P1") inside a
// colored pill; the muted default is shared by all three maps.

export const FILLED_BADGE_MUTED_CLASS =
  "rounded bg-[var(--bg-tertiary)] px-2 py-0.5 text-xs font-semibold text-[var(--text-primary)]";

const FILLED_BADGE_BASE = "rounded px-2 py-0.5 text-xs font-semibold text-white";

export function incidentPriorityBadgeClass(priority: string): string {
  switch (priority) {
    case "P1":
      return `${FILLED_BADGE_BASE} bg-red-500`;
    case "P2":
      return `${FILLED_BADGE_BASE} bg-orange-500`;
    case "P3":
      return `${FILLED_BADGE_BASE} bg-amber-500`;
    case "P4":
      return `${FILLED_BADGE_BASE} bg-blue-500`;
    case "P5":
      return `${FILLED_BADGE_BASE} bg-slate-500`;
    default:
      return FILLED_BADGE_MUTED_CLASS;
  }
}

export function incidentSeverityBadgeClass(severity: string): string {
  switch (severity) {
    case "critical":
      return `${FILLED_BADGE_BASE} bg-red-500`;
    case "high":
      return `${FILLED_BADGE_BASE} bg-orange-500`;
    case "warning":
      return `${FILLED_BADGE_BASE} bg-amber-500`;
    case "info":
      return `${FILLED_BADGE_BASE} bg-sky-500`;
    default:
      return FILLED_BADGE_MUTED_CLASS;
  }
}

export function incidentImpactBadgeClass(impact: string): string {
  switch (impact) {
    case "high":
      return `${FILLED_BADGE_BASE} bg-red-500`;
    case "medium":
      return `${FILLED_BADGE_BASE} bg-amber-500`;
    case "low":
      return `${FILLED_BADGE_BASE} bg-sky-500`;
    default:
      return FILLED_BADGE_MUTED_CLASS;
  }
}
