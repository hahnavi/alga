import { MessageSquare } from "@lucide/vue";
import type { Component } from "vue";
import type { IntegrationInfo } from "./api";

export type ProviderId = "mattermost" | "slack" | "twilio" | "telnyx";

export type ProviderStatus = {
  label: string;
  cls: "badge-green" | "badge-yellow" | "badge-muted" | "badge-red";
  /** When true the provider card should appear dimmed (inactive or coming soon). */
  dimmed: boolean;
  /** When true the provider supports opening a Configure dialog. */
  configurable: boolean;
};

/** Decorative brand color tokens applied to provider icon backgrounds and accents. */
export const PROVIDER_BRAND: Record<
  ProviderId,
  { bgClass: string; accentClass: string; iconClass: string }
> = {
  mattermost: {
    bgClass: "bg-[var(--bg-badge-warning)]",
    accentClass: "bg-transparent",
    iconClass: "text-[var(--text-primary)]",
  },
  slack: {
    bgClass: "bg-[#4A154B]/10",
    accentClass: "bg-[#4A154B]/40",
    iconClass: "text-[#4A154B]",
  },
  twilio: {
    bgClass: "bg-red-500/10",
    accentClass: "bg-red-500/40",
    iconClass: "text-red-500",
  },
  telnyx: {
    bgClass: "bg-emerald-500/10",
    accentClass: "bg-emerald-500/40",
    iconClass: "text-emerald-500",
  },
};

/** Slack and Mattermost use a brand image; voice providers use a Phone glyph. */
export const PROVIDER_ICON_IS_IMAGE: Record<ProviderId, boolean> = {
  mattermost: true,
  slack: true,
  twilio: false,
  telnyx: false,
};

export const PROVIDER_FALLBACK_ICON: Component = MessageSquare;

export type ProviderMeta = {
  id: ProviderId;
  /** Human label shown on the card header and dialog title. */
  label: string;
  /** Short "what is this" caption shown on the card when there's no other state. */
  description: string;
};

export const PROVIDERS: Record<ProviderId, ProviderMeta> = {
  mattermost: {
    id: "mattermost",
    label: "Mattermost",
    description: "Mattermost support is on the roadmap.",
  },
  slack: {
    id: "slack",
    label: "Slack",
    description:
      "Slack delivers alerts to a channel and supports two-way replies with on-call agents.",
  },
  twilio: {
    id: "twilio",
    label: "Twilio Voice",
    description: "Automated voice call escalation for critical alerts.",
  },
  telnyx: {
    id: "telnyx",
    label: "Telnyx Voice",
    description: "Automated voice call escalation via Telnyx.",
  },
};

function voiceProviderStatus(
  _id: ProviderId,
  voice: { active: boolean; enabled: boolean; provider_enabled: boolean } | undefined,
): ProviderStatus {
  if (!voice || !voice.active) {
    return { label: "Inactive", cls: "badge-muted", dimmed: true, configurable: false };
  }
  if (!voice.enabled) {
    return { label: "Not configured", cls: "badge-muted", dimmed: false, configurable: true };
  }
  if (!voice.provider_enabled) {
    return { label: "Disabled", cls: "badge-yellow", dimmed: false, configurable: true };
  }
  return { label: "Enabled", cls: "badge-green", dimmed: false, configurable: true };
}

export function providerStatus(id: ProviderId, info: IntegrationInfo | null): ProviderStatus {
  if (!info) {
    return { label: "Unknown", cls: "badge-muted", dimmed: false, configurable: false };
  }
  switch (id) {
    case "mattermost":
      return { label: "Coming soon", cls: "badge-yellow", dimmed: true, configurable: false };
    case "slack": {
      const s = info.slack;
      if (!s.enabled)
        return { label: "Not configured", cls: "badge-muted", dimmed: false, configurable: true };
      if (!s.provider_enabled)
        return { label: "Paused", cls: "badge-yellow", dimmed: false, configurable: true };
      return { label: "Active", cls: "badge-green", dimmed: false, configurable: true };
    }
    case "twilio":
      return voiceProviderStatus(id, info.twilio);
    case "telnyx":
      return voiceProviderStatus(id, info.telnyx);
  }
}
