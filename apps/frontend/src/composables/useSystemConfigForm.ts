import { ref } from "vue";
import { api, type SystemConfigValues } from "@/lib/api";
import { useAsyncData } from "@/composables/useAsyncData";
import { useFormSubmit } from "@/composables/useFormSubmit";

export type SummaryIntervals = Record<string, string>;

export type SystemForm = {
  correlation_window: string;
  correlation_cooldown_ttl: string;
  investigation_timeout: string;
  max_concurrent_investigations: number;
  agent_presence_ttl: string;
  agent_disconnect_grace: string;
  scheduler_leader_ttl: string;
  session_expiry_hours: number;
  log_level: string;
  slack_incident_channels_enabled: boolean;
  slack_incident_channel_visibility: string;
  slack_incident_channel_trigger_status: string;
  slack_incident_channel_archive_on_close: boolean;
  incident_summary_enabled: boolean;
  incident_summary_interval: string;
  incident_summary_intervals: SummaryIntervals;

  google_oauth_enabled: boolean;
  google_client_id: string;
  google_client_secret: string;
  google_client_secret_set: boolean;
  google_oauth_redirect_url: string;
};

export const SUMMARY_SEVERITIES = ["critical", "high", "warning", "info"] as const;

const DEFAULTS: SystemForm = {
  correlation_window: "",
  correlation_cooldown_ttl: "",
  investigation_timeout: "",
  max_concurrent_investigations: 3,
  agent_presence_ttl: "",
  agent_disconnect_grace: "",
  scheduler_leader_ttl: "",
  session_expiry_hours: 24,
  log_level: "info",
  slack_incident_channels_enabled: false,
  slack_incident_channel_visibility: "private",
  slack_incident_channel_trigger_status: "active",
  slack_incident_channel_archive_on_close: true,
  incident_summary_enabled: false,
  incident_summary_interval: "",
  incident_summary_intervals: {},

  google_oauth_enabled: true,
  google_client_id: "",
  google_client_secret: "",
  google_client_secret_set: false,
  google_oauth_redirect_url: "",
};

export function intervalsFromConfig(raw: Record<string, string> | undefined): SummaryIntervals {
  const out: SummaryIntervals = {};
  for (const sev of SUMMARY_SEVERITIES) {
    const v = raw?.[sev];
    out[sev] = typeof v === "string" ? v : "";
  }
  return out;
}

export function intervalsEqual(a: SummaryIntervals, b: SummaryIntervals): boolean {
  const keys = new Set<string>([...Object.keys(a), ...Object.keys(b)]);
  for (const key of keys) {
    if ((a[key] ?? "") !== (b[key] ?? "")) return false;
  }
  return true;
}

export function serializeIntervals(m: SummaryIntervals): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [sev, val] of Object.entries(m)) {
    const trimmed = val.trim();
    if (trimmed !== "") out[sev] = trimmed;
  }
  return out;
}

function fromConfig(cfg: SystemConfigValues): SystemForm {
  return {
    correlation_window: cfg.correlation_window ?? "",
    correlation_cooldown_ttl: cfg.correlation_cooldown_ttl ?? "",
    investigation_timeout: cfg.investigation_timeout ?? "",
    max_concurrent_investigations: cfg.max_concurrent_investigations ?? 3,
    agent_presence_ttl: cfg.agent_presence_ttl ?? "",
    agent_disconnect_grace: cfg.agent_disconnect_grace ?? "",
    scheduler_leader_ttl: cfg.scheduler_leader_ttl ?? "",
    session_expiry_hours: cfg.session_expiry_hours ?? 24,
    log_level: cfg.log_level || "info",
    slack_incident_channels_enabled: cfg.slack_incident_channels_enabled ?? false,
    slack_incident_channel_visibility: cfg.slack_incident_channel_visibility || "private",
    slack_incident_channel_trigger_status: cfg.slack_incident_channel_trigger_status || "active",
    slack_incident_channel_archive_on_close: cfg.slack_incident_channel_archive_on_close ?? true,
    incident_summary_enabled: cfg.incident_summary_enabled ?? false,
    incident_summary_interval: cfg.incident_summary_interval ?? "",
    incident_summary_intervals: intervalsFromConfig(cfg.incident_summary_intervals),

    google_oauth_enabled: cfg.google_oauth_enabled ?? true,
    google_client_id: cfg.google_client_id ?? "",
    google_client_secret: "",
    google_client_secret_set: cfg.google_client_secret_set ?? false,
    google_oauth_redirect_url: cfg.google_oauth_redirect_url ?? "",
  };
}

/**
 * Shared state and helpers for the split system-config pages. Each page owns a
 * subset of fields; `changedFields`/`isDirty`/`discard` operate on a field list
 * so the load, save, and dirty-tracking logic lives in one place.
 */
export function useSystemConfigForm() {
  const form = ref<SystemForm>({ ...DEFAULTS });
  const original = ref<SystemForm | null>(null);
  const environment = ref("");
  const updatedAt = ref("");
  const isSlackConfigured = ref(false);

  const {
    loading,
    error,
    reload: loadConfig,
  } = useAsyncData(async () => {
    const cfg = await api.getSystemConfig();
    form.value = fromConfig(cfg);
    original.value = {
      ...form.value,
      incident_summary_intervals: { ...form.value.incident_summary_intervals },
    };
    environment.value = cfg.environment ?? "";
    updatedAt.value = cfg.updated_at ?? "";
    return cfg;
  }, "Failed to load system config");

  const { submitting: saving, formError: saveError, withSubmit: withSave } = useFormSubmit();

  function isDirty(fields: ReadonlyArray<keyof SystemForm>): boolean {
    const orig = original.value;
    if (!orig) return false;
    return fields.some((f) => form.value[f] !== orig[f]);
  }

  function changedFields(fields: ReadonlyArray<keyof SystemForm>): Record<string, unknown> {
    const orig = original.value;
    if (!orig) return {};
    const payload: Record<string, unknown> = {};
    for (const f of fields) {
      if (form.value[f] !== orig[f]) payload[f] = form.value[f];
    }
    return payload;
  }

  function discard(fields: ReadonlyArray<keyof SystemForm>): void {
    const orig = original.value;
    if (!orig) return;
    const next: SystemForm = { ...form.value };
    for (const f of fields) {
      Object.assign(next, { [f]: orig[f] });
    }
    form.value = next;
  }

  async function save(payload: Record<string, unknown>, label: string): Promise<void> {
    if (Object.keys(payload).length === 0) return;
    await withSave(async () => {
      await api.updateSystemConfig(payload as Partial<SystemConfigValues>);
      await loadConfig();
    }, `${label} settings saved`);
  }

  function loadSlackStatus(): void {
    api
      .getIntegrations()
      .then((integrations) => {
        isSlackConfigured.value = integrations?.slack?.provider_enabled ?? false;
      })
      .catch(() => {
        isSlackConfigured.value = false;
      });
  }

  return {
    form,
    original,
    environment,
    updatedAt,
    isSlackConfigured,
    loading,
    error,
    loadConfig,
    saving,
    saveError,
    isDirty,
    changedFields,
    discard,
    save,
    loadSlackStatus,
  };
}
