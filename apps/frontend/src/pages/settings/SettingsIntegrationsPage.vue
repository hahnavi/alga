<script setup lang="ts">
import { onMounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import Card from "@/components/ui/Card.vue";
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
  <section class="px-4 py-4 md:px-6 md:py-6">
    <Card class="mx-auto max-w-2xl">
      <div class="space-y-4">
        <SettingsIntegrationsTab />
      </div>
    </Card>
  </section>
</template>
