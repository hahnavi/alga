<script setup lang="ts">
import { Settings } from "@lucide/vue";
import { computed } from "vue";
import {
  PROVIDER_BRAND,
  PROVIDER_ICON_IS_IMAGE,
  PROVIDER_FALLBACK_ICON,
  PROVIDERS,
  type ProviderId,
  type ProviderStatus,
} from "@/lib/integrations";
import { getProviderIconSrc } from "@/lib/providerIcon";
import Button from "@/components/ui/Button.vue";

defineOptions({ name: "ProviderCard" });

const props = defineProps<{
  providerId: ProviderId;
  status: ProviderStatus;
  /** When true, surface the Configure button even if the status says no. */
  allowConfigure?: boolean;
  /** Extra detail lines rendered beneath the header — each is `{ icon, text }`. */
  details?: Array<{ icon?: unknown; text: string }>;
}>();

defineEmits<{
  configure: [];
}>();

const meta = computed(() => PROVIDERS[props.providerId]);
const brand = computed(() => PROVIDER_BRAND[props.providerId]);
const isImageIcon = computed(() => PROVIDER_ICON_IS_IMAGE[props.providerId]);

function disabledConfigureTitle(): string {
  if (props.status.label === "Coming soon") return "Mattermost integration is coming soon";
  return "Configure";
}
</script>

<template>
  <div
    class="group relative overflow-hidden rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] transition-all duration-200"
    :class="[status.dimmed ? 'opacity-70' : 'hover:border-[var(--border-secondary)]']"
  >
    <div
      class="pointer-events-none absolute inset-x-0 top-0 h-px"
      :class="status.cls === 'badge-green' ? brand.accentClass : 'bg-transparent'"
    />
    <div class="p-4">
      <div class="mb-3 flex items-start justify-between gap-3">
        <div class="flex min-w-0 items-center gap-3">
          <div
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
            :class="brand.bgClass"
          >
            <img
              v-if="isImageIcon"
              :src="getProviderIconSrc(providerId)"
              :alt="meta.label"
              class="h-5 w-5 rounded-sm"
              loading="lazy"
              decoding="async"
            />
            <component
              :is="PROVIDER_FALLBACK_ICON"
              v-else
              class="h-5 w-5"
              :class="brand.iconClass"
            />
          </div>
          <div class="min-w-0">
            <h4 class="truncate font-medium text-[var(--text-primary)]">
              {{ meta.label }}
            </h4>
            <span class="mt-0.5 inline-block" :class="['badge', status.cls]">
              {{ status.label }}
            </span>
          </div>
        </div>
        <Button
          v-if="(allowConfigure ?? true) || status.configurable"
          size="sm"
          :disabled="!status.configurable"
          :title="disabledConfigureTitle()"
          :aria-label="`Configure ${meta.label}`"
          @click="$emit('configure')"
        >
          <Settings class="h-4 w-4" />
        </Button>
      </div>

      <div v-if="details && details.length" class="space-y-1.5">
        <div
          v-for="(line, i) in details"
          :key="i"
          class="flex items-center gap-1.5 text-xs text-[var(--text-muted)]"
        >
          <component :is="line.icon" v-if="line.icon" class="h-3 w-3 shrink-0" />
          <span class="truncate">{{ line.text }}</span>
        </div>
      </div>
      <p v-else class="text-xs text-[var(--text-muted)]">{{ meta.description }}</p>
    </div>
  </div>
</template>
