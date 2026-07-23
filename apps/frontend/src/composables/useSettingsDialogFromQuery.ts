import { ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { queryString } from "@/lib/routing";
import type { SettingsTabId } from "@/components/ui/settings/SettingsDialog.vue";

function settingTabFromQuery(tab: string): SettingsTabId {
  if (tab === "general" || tab === "appearance" || tab === "security" || tab === "integrations") {
    return tab;
  }
  return "general";
}

/**
 * Owns the `?settings=…` / `?slack_linked=…` / `?google_linked=…` query
 * string → Settings dialog mapping. When any of these query params is
 * present, the dialog opens to the right tab; on close the params are
 * stripped (via `router.replace`) so a refresh doesn't reopen it.
 *
 * Slack/Google linked events also re-fetch the current user so the
 * page header reflects the new linked state immediately.
 */
export function useSettingsDialogFromQuery() {
  const route = useRoute();
  const router = useRouter();
  const auth = useAuthStore();

  const showSettings = ref(false);
  const settingsTab = ref<SettingsTabId>("general");

  function openSettings(tab: SettingsTabId = "general") {
    settingsTab.value = tab;
    showSettings.value = true;
  }

  function closeSettings() {
    showSettings.value = false;
    if (
      route.query.settings ||
      route.query.slack_linked ||
      route.query.google_linked ||
      route.query.message
    ) {
      const query = { ...route.query };
      delete query.settings;
      delete query.slack_linked;
      delete query.google_linked;
      delete query.message;
      void router.replace({ path: route.path, query });
    }
  }

  watch(
    () => [route.query.settings, route.query.slack_linked, route.query.google_linked] as const,
    ([settings, slackLinked, googleLinked]) => {
      if (!settings && !slackLinked && !googleLinked) return;
      const targetTab =
        slackLinked || googleLinked
          ? "integrations"
          : settingTabFromQuery(queryString(route.query, "settings"));
      openSettings(targetTab);
      if (slackLinked || googleLinked) void auth.fetchCurrentUser();
    },
    { immediate: true },
  );

  return { showSettings, settingsTab, openSettings, closeSettings };
}
