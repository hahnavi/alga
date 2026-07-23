<script setup lang="ts">
import { ref, computed } from "vue";
import { Copy, Eye, EyeOff } from "@lucide/vue";
import Button from "@/components/ui/Button.vue";

const props = defineProps<{
  secret: string;
}>();

const emit = defineEmits<{
  copy: [];
}>();

const revealed = ref(false);

const maskedSecret = computed(() => {
  if (props.secret.length <= 12) return "••••••••";
  return props.secret.slice(0, 4) + "••••" + props.secret.slice(-4);
});
</script>

<template>
  <div
    class="flex items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-code)] p-3"
  >
    <code class="min-w-0 flex-1 break-all text-xs text-[var(--text-code)]">{{
      revealed ? secret : maskedSecret
    }}</code>
    <Button size="sm" type="button" @click="revealed = !revealed">
      <EyeOff v-if="revealed" class="h-3.5 w-3.5" />
      <Eye v-else class="h-3.5 w-3.5" />
    </Button>
    <Button size="sm" type="button" @click="emit('copy')">
      <Copy class="h-3.5 w-3.5" />
    </Button>
  </div>
</template>
