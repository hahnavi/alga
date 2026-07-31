<script setup lang="ts">
import AuthLayout from "@/components/ui/AuthLayout.vue";

defineOptions({ name: "AuthShell" });
</script>

<template>
  <AuthLayout>
    <router-view v-slot="{ Component }">
      <Transition name="auth-swap" mode="out-in">
        <component :is="Component" />
      </Transition>
    </router-view>
  </AuthLayout>
</template>
