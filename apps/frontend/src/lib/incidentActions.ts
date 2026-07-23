import { ExternalLink, Siren, Trash2, type LucideIcon } from "@lucide/vue";
import type { IncidentStatus } from "@/lib/api";

export type IncidentMenuItem = {
  label: string;
  icon?: LucideIcon;
  onSelect: () => void;
  destructive?: boolean;
  disabled?: boolean;
  href?: string;
  target?: "_blank";
};

export type IncidentMenuContext = {
  canCommand: boolean;
  canDelete: boolean;
  conferenceHref: string | null;
  loading: boolean;
  escalating: boolean;
};

type Emitters = {
  edit: () => void;
  escalate: () => void;
  acknowledge: () => void;
  mitigate: () => void;
  resolve: () => void;
  close: () => void;
  reopen: () => void;
  cancel: () => void;
  promote: () => void;
  delete: () => void;
};

const COMMAND_STATUSES = new Set<IncidentStatus>([
  "detected",
  "triaging",
  "active",
  "mitigated",
  "resolved",
  "closed",
]);

function commandActionsFor(
  status: IncidentStatus,
  ctx: IncidentMenuContext,
  emit: Emitters,
): IncidentMenuItem[] {
  if (!ctx.canCommand) return [];
  const items: IncidentMenuItem[] = [];
  const busy = ctx.loading;
  switch (status) {
    case "detected":
      items.push({ label: "Acknowledge", onSelect: emit.acknowledge, disabled: busy });
      items.push({ label: "Cancel", onSelect: emit.cancel, destructive: true });
      break;
    case "triaging":
      items.push({ label: "Start Response", onSelect: emit.promote, disabled: busy });
      items.push({ label: "Acknowledge", onSelect: emit.acknowledge, disabled: busy });
      items.push({ label: "Mitigate", onSelect: emit.mitigate, disabled: busy });
      items.push({ label: "Cancel", onSelect: emit.cancel, destructive: true });
      break;
    case "active":
      items.push({ label: "Mitigate", onSelect: emit.mitigate, disabled: busy });
      items.push({ label: "Resolve", onSelect: emit.resolve, disabled: busy });
      items.push({ label: "Cancel", onSelect: emit.cancel, destructive: true });
      break;
    case "mitigated":
      items.push({ label: "Resolve", onSelect: emit.resolve, disabled: busy });
      items.push({ label: "Close", onSelect: emit.close, disabled: busy });
      break;
    case "resolved":
      items.push({ label: "Close", onSelect: emit.close, disabled: busy });
      items.push({ label: "Reopen", onSelect: emit.reopen, disabled: busy });
      break;
    case "closed":
      items.push({ label: "Reopen", onSelect: emit.reopen, disabled: busy });
      break;
  }
  return items;
}

/**
 * Build the menu items for an incident given its status, the caller's
 * permissions, and an emitter object. The conference link, if any, is
 * rendered as a non-destructive external link item (the caller decides
 * navigation by setting `href`).
 */
export function incidentMenuItemsFor(
  status: IncidentStatus,
  ctx: IncidentMenuContext,
  emit: Emitters,
): IncidentMenuItem[] {
  const items: IncidentMenuItem[] = [];

  items.push({ label: "Edit", onSelect: emit.edit });

  if (ctx.canCommand && (status === "active" || status === "triaging")) {
    items.push({
      label: ctx.escalating ? "Escalating…" : "Escalate",
      icon: Siren,
      onSelect: emit.escalate,
      disabled: ctx.escalating,
    });
  }

  if (ctx.conferenceHref) {
    items.push({
      label: "Conference",
      icon: ExternalLink,
      onSelect: () => {
        /* nav handled by anchor target */
      },
      href: ctx.conferenceHref,
      target: "_blank",
    });
  }

  if (COMMAND_STATUSES.has(status) && ctx.canCommand) {
    items.push(...commandActionsFor(status, ctx, emit));
  }

  if (ctx.canDelete) {
    items.push({
      label: "Delete",
      icon: Trash2,
      onSelect: emit.delete,
      destructive: true,
    });
  }

  return items;
}

/** A subset of menu items for the incident list row (no edit/escalate). */
export function incidentListMenuItemsFor(
  status: IncidentStatus,
  ctx: IncidentMenuContext,
  emit: Pick<
    Emitters,
    "reopen" | "resolve" | "close" | "delete" | "acknowledge" | "mitigate" | "cancel"
  >,
): IncidentMenuItem[] {
  const items = commandActionsFor(status, ctx, {
    edit: () => {},
    escalate: () => {},
    acknowledge: emit.acknowledge,
    mitigate: emit.mitigate,
    resolve: emit.resolve,
    close: emit.close,
    reopen: emit.reopen,
    cancel: emit.cancel,
    promote: () => {},
    delete: () => {},
  });
  if (ctx.canDelete) {
    items.push({
      label: "Delete",
      icon: Trash2,
      onSelect: emit.delete,
      destructive: true,
    });
  }
  return items;
}
