<script setup lang="ts">
import { useRouter } from "vue-router";
import Button from "@/components/ui/Button.vue";

defineOptions({ name: "NotFoundPage" });

const router = useRouter();
</script>

<template>
  <div class="flex flex-col items-center justify-center px-4 py-24">
    <h1 class="text-4xl font-bold text-[var(--text-primary)]">404</h1>
    <p class="mt-2 text-[var(--text-muted)]">Page not found</p>
    <Button class="mt-6" @click="router.back()">Go Back</Button>
  </div>
</template>
