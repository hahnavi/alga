import { ref, watch, type Ref } from "vue";
import {
  api,
  type AlertInvestigationRecord,
  type AlertRecord,
  type RelatedAlert,
  type RelatedIncident,
} from "@/lib/api";
import { useAsyncData } from "@/composables/useAsyncData";
import { getErrorMessage } from "@/lib/error";

/**
 * Owns the load pipeline for the alert detail page: alert + its
 * investigation + related alerts / incident. Splits the previously
 * bundled `useAsyncData` so the page doesn't have to know that
 * `getAlert` returns the investigation embedded, and so the related
 * data has its own loader (it's not part of the canonical alert row).
 *
 * Returns the loaders' state as flat refs so the page can use them
 * without unwrapping an extra layer.
 */
export function useAlertDetailData(alertNumber: Ref<number>) {
  const investigation = ref<AlertInvestigationRecord | null>(null);
  const relatedAlerts = ref<RelatedAlert[]>([]);
  const relatedIncident = ref<RelatedIncident | null>(null);

  const {
    data: alert,
    loading,
    error,
    reload: load,
  } = useAsyncData(async () => {
    const data = await api.getAlert(alertNumber.value);
    investigation.value = data.alert_investigation ?? null;
    return data.alert as AlertRecord;
  }, "Failed to load alert");

  async function loadRelated() {
    try {
      const data = await api.getAlertRelated(alertNumber.value);
      relatedAlerts.value = data.related_alerts ?? [];
      relatedIncident.value = data.incident ?? null;
    } catch (err) {
      relatedAlerts.value = [];
      relatedIncident.value = null;
      // Keep the toast-only behavior of the previous inline loader.
      getErrorMessage(err, "Failed to load related alerts");
    }
  }

  async function silentReload() {
    try {
      const data = await api.getAlert(alertNumber.value);
      investigation.value = data.alert_investigation ?? null;
      return data.alert as AlertRecord;
    } catch {
      return null;
    }
  }

  // Re-fetch related whenever the route's alert number changes.
  watch(
    alertNumber,
    (next) => {
      if (!Number.isFinite(next)) return;
      void loadRelated();
    },
    { immediate: false },
  );

  return {
    alert,
    investigation,
    relatedAlerts,
    relatedIncident,
    loading,
    error,
    load,
    loadRelated,
    silentReload,
  };
}
