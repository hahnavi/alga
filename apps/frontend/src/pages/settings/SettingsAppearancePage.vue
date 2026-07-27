<script setup lang="ts">
import Card from "@/components/ui/Card.vue";
import SettingsAppearanceTab from "@/components/ui/settings/SettingsAppearanceTab.vue";

defineOptions({ name: "SettingsAppearancePage" });
</script>

<template>
  <section class="px-4 py-4 md:px-6 md:py-6">
    <Card class="mx-auto max-w-2xl">
      <div class="space-y-4">
        <SettingsAppearanceTab />
      </div>
    </Card>
  </section>
</template>
