<script setup lang="ts">
import { User } from "@lucide/vue";
import SettingsPageShell from "@/components/ui/settings/SettingsPageShell.vue";
import SettingsGeneralTab from "@/components/ui/settings/SettingsGeneralTab.vue";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";

defineOptions({ name: "SettingsGeneralPage" });

usePageHeaderActions({
  title: "General",
  titleIcon: User,
  showAdd: false,
});
</script>

<template>
  <SettingsPageShell description="Your profile and account details.">
    <SettingsGeneralTab />
  </SettingsPageShell>
</template>
