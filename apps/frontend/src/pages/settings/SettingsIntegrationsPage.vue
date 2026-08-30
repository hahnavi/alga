<script setup lang="ts">
import { onMounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import SettingsPageShell from "@/components/ui/settings/SettingsPageShell.vue";
import SettingsIntegrationsTab from "@/components/ui/settings/SettingsIntegrationsTab.vue";
import { useAuthStore } from "@/stores/auth";

defineOptions({ name: "SettingsIntegrationsPage" });

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

// OAuth link callbacks land here as /settings?slack_linked=… or
// ?google_linked=… (redirected by the /settings route). Refresh the
// user so linked state is current, then strip the one-shot params.
onMounted(() => {
  if (!route.query.slack_linked && !route.query.google_linked) return;
  void auth.fetchCurrentUser();
  const query = { ...route.query };
  delete query.slack_linked;
  delete query.google_linked;
  delete query.message;
  void router.replace({ path: route.path, query });
});
</script>

<template>
  <SettingsPageShell description="Linked Slack and Google accounts.">
    <SettingsIntegrationsTab />
  </SettingsPageShell>
</template>
