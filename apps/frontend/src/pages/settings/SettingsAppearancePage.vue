<script setup lang="ts">
import SettingsPageShell from "@/components/ui/settings/SettingsPageShell.vue";
import SettingsAppearanceTab from "@/components/ui/settings/SettingsAppearanceTab.vue";

defineOptions({ name: "SettingsAppearancePage" });
</script>

<template>
  <SettingsPageShell description="Theme and font preferences apply to this browser.">
    <SettingsAppearanceTab />
  </SettingsPageShell>
</template>
