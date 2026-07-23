<script setup lang="ts">
import { computed, ref, watch } from "vue";
import DialogCloseButton from "@/components/ui/DialogCloseButton.vue";
import Tabs, { type Tab } from "@/components/ui/Tabs.vue";
import { useModalFocusTrap } from "@/lib/dialogFocus";
import { useEscapeKey } from "@/composables/useEscapeKey";
import SettingsGeneralTab from "./SettingsGeneralTab.vue";
import SettingsAppearanceTab from "./SettingsAppearanceTab.vue";
import SettingsSecurityTab from "./SettingsSecurityTab.vue";
import SettingsIntegrationsTab from "./SettingsIntegrationsTab.vue";

export type SettingsTabId = "general" | "appearance" | "security" | "integrations";

const props = withDefaults(
  defineProps<{
    open?: boolean;
    tab?: SettingsTabId;
  }>(),
  {
    open: false,
    tab: "general",
  },
);

const emit = defineEmits<{
  close: [];
}>();

const dialogRef = ref<HTMLElement | null>(null);
const openRef = computed(() => props.open);

useModalFocusTrap(openRef, () => dialogRef.value);

const settingsTab = ref<SettingsTabId>(props.tab);
const tabItems: Tab<SettingsTabId>[] = [
  { id: "general", label: "General" },
  { id: "appearance", label: "Appearance" },
  { id: "security", label: "Security" },
  { id: "integrations", label: "Integrations" },
];

watch(
  () => [props.open, props.tab] as const,
  ([val, tab]) => {
    if (val) {
      settingsTab.value = tab;
    }
  },
);

function closeSettings() {
  emit("close");
}

useEscapeKey(closeSettings, () => props.open);
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 max-sm:p-0"
      @click.self="closeSettings"
    >
      <div
        ref="dialogRef"
        role="dialog"
        aria-modal="true"
        aria-label="Settings"
        class="flex w-full max-w-lg flex-col overflow-hidden rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] p-5 text-[var(--text-primary)] shadow-xl h-[640px] sm:rounded-lg max-sm:fixed max-sm:inset-0 max-sm:h-full max-sm:max-w-none max-sm:rounded-none"
      >
        <div class="mb-3 flex items-center justify-between">
          <h3 class="text-base font-semibold">Settings</h3>
          <DialogCloseButton :on-click="closeSettings" size="md" />
        </div>

        <div
          class="flex min-h-0 flex-1 flex-col [&_[role=tabpanel]]:min-h-0 [&_[role=tabpanel]]:flex-1 [&_[role=tabpanel]]:space-y-4 [&_[role=tabpanel]]:overflow-y-auto [&_[role=tabpanel]]:pt-4"
        >
          <Tabs
            v-model="settingsTab"
            :tabs="tabItems"
            aria-label="Settings sections"
            id-prefix="settings"
          >
            <template #panel-general>
              <SettingsGeneralTab :on-close="closeSettings" />
            </template>
            <template #panel-appearance>
              <SettingsAppearanceTab :on-close="closeSettings" />
            </template>
            <template #panel-security>
              <SettingsSecurityTab :on-close="closeSettings" />
            </template>
            <template #panel-integrations>
              <SettingsIntegrationsTab :on-close="closeSettings" />
            </template>
          </Tabs>
        </div>
      </div>
    </div>
  </Teleport>
</template>
