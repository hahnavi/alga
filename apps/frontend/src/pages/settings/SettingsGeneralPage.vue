<script setup lang="ts">
import SettingsPageShell from "@/components/ui/settings/SettingsPageShell.vue";
import SettingsGeneralTab from "@/components/ui/settings/SettingsGeneralTab.vue";

defineOptions({ name: "SettingsGeneralPage" });
</script>

<template>
  <SettingsPageShell description="Your profile and account details.">
    <SettingsGeneralTab />
  </SettingsPageShell>
</template>
