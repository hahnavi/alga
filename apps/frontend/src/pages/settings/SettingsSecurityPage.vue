<script setup lang="ts">
import Card from "@/components/ui/Card.vue";
import SettingsSecurityTab from "@/components/ui/settings/SettingsSecurityTab.vue";

defineOptions({ name: "SettingsSecurityPage" });
</script>

<template>
  <section class="px-4 py-4 md:px-6 md:py-6">
    <Card class="mx-auto max-w-2xl">
      <div class="space-y-4">
        <SettingsSecurityTab />
      </div>
    </Card>
  </section>
</template>
