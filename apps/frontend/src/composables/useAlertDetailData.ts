import { ref, type Ref } from "vue";
import {
  api,
  type AlertInvestigationRecord,
  type RelatedAlert,
  type RelatedIncident,
} from "@/lib/api";
import { useAsyncData } from "@/composables/useAsyncData";
import { getErrorMessage } from "@/lib/error";
import { useToast } from "@/lib/toast";

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
  const { push } = useToast();
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
    return data.alert;
  }, "Failed to load alert");

  async function loadRelated() {
    try {
      const data = await api.getAlertRelated(alertNumber.value);
      relatedAlerts.value = data.related_alerts ?? [];
      relatedIncident.value = data.incident ?? null;
    } catch (err) {
      relatedAlerts.value = [];
      relatedIncident.value = null;
      push(getErrorMessage(err, "Failed to load related alerts"), "error");
    }
  }

  async function silentReload() {
    try {
      const data = await api.getAlert(alertNumber.value);
      investigation.value = data.alert_investigation ?? null;
      alert.value = data.alert;
      return data.alert;
    } catch {
      // Silent by design: SSE-triggered refreshes must not error-toast when
      // the alert was deleted or the request transiently fails; the next
      // event or navigation retries.
      return null;
    }
  }

  // The caller owns when related data loads (initial mount + route change +
  // SSE-triggered refreshes) — both the page's onMounted and its alertNumber
  // watcher trigger `loadRelated`, so an extra internal watch would double-fetch.

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
