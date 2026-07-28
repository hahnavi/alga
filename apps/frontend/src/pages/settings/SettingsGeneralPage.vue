<script setup lang="ts">
import Card from "@/components/ui/Card.vue";
import SettingsGeneralTab from "@/components/ui/settings/SettingsGeneralTab.vue";

defineOptions({ name: "SettingsGeneralPage" });
</script>

<template>
  <section class="px-4 py-4 md:px-6 md:py-6">
    <Card class="mx-auto max-w-2xl">
      <div class="space-y-4">
        <SettingsGeneralTab />
      </div>
    </Card>
  </section>
</template>
