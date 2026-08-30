<script setup lang="ts">
import { Palette } from "@lucide/vue";
import SettingsPageShell from "@/components/ui/settings/SettingsPageShell.vue";
import SettingsAppearanceTab from "@/components/ui/settings/SettingsAppearanceTab.vue";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";

defineOptions({ name: "SettingsAppearancePage" });

usePageHeaderActions({
  title: "Appearance",
  titleIcon: Palette,
  showAdd: false,
});
</script>

<template>
  <SettingsPageShell description="Theme and font preferences apply to this browser.">
    <SettingsAppearanceTab />
  </SettingsPageShell>
</template>
