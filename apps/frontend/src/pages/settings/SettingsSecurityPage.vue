<script setup lang="ts">
import { Lock } from "@lucide/vue";
import SettingsPageShell from "@/components/ui/settings/SettingsPageShell.vue";
import SettingsSecurityTab from "@/components/ui/settings/SettingsSecurityTab.vue";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";

defineOptions({ name: "SettingsSecurityPage" });

usePageHeaderActions({
  title: "Security",
  titleIcon: Lock,
  showAdd: false,
});
</script>

<template>
  <SettingsPageShell description="Manage your password and sign-in security.">
    <SettingsSecurityTab />
  </SettingsPageShell>
</template>
