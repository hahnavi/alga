<script setup lang="ts">
import SettingsPageShell from "@/components/ui/settings/SettingsPageShell.vue";
import SettingsSecurityTab from "@/components/ui/settings/SettingsSecurityTab.vue";

defineOptions({ name: "SettingsSecurityPage" });
</script>

<template>
  <SettingsPageShell description="Manage your password and sign-in security.">
    <SettingsSecurityTab />
  </SettingsPageShell>
</template>
