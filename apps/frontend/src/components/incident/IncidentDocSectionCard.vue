<script setup lang="ts">
import { Pencil, Save, type LucideIcon } from "@lucide/vue";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import MarkdownEditor from "@/components/ui/MarkdownEditor.vue";
import MarkdownRenderer from "@/components/ui/MarkdownRenderer.vue";
import { CARD_ICON_BTN_CLASS } from "@/lib/uiClasses";
import type { AgentTokenRow, UserInfo } from "@/lib/api";

defineOptions({ name: "IncidentDocSectionCard" });

/**
 * One editable incident document surface (summary / root cause /
 * resolution / impact assessment): icon + title, edit affordance,
 * markdown editor while editing, rendered markdown otherwise.
 */
const props = defineProps<{
  icon: LucideIcon;
  title: string;
  content: string;
  editing: boolean;
  saving: boolean;
  canEdit: boolean;
  emptyText: string;
  placeholder: string;
  users: UserInfo[];
  agents: AgentTokenRow[];
}>();

const emit = defineEmits<{
  "start-edit": [];
  "cancel-edit": [];
  save: [];
  "update:content": [value: string];
}>();

function onInput(value: string) {
  emit("update:content", value);
}
</script>

<template>
  <Card class="hover:shadow-md transition-all duration-300">
    <div
      class="mb-3 flex items-center justify-between gap-2 border-b border-[var(--border-primary)] pb-2"
    >
      <div class="flex items-center gap-2">
        <component :is="props.icon" class="h-4 w-4 text-[var(--text-secondary)]" />
        <h3 class="text-sm font-semibold text-[var(--text-primary)]">
          {{ props.title }}
        </h3>
      </div>
      <button
        v-if="props.canEdit && !props.editing"
        type="button"
        :class="CARD_ICON_BTN_CLASS"
        :title="`Edit ${props.title.toLowerCase()}`"
        @click="emit('start-edit')"
      >
        <Pencil class="h-3.5 w-3.5" />
      </button>
    </div>
    <MarkdownEditor
      v-if="props.editing"
      :model-value="props.content"
      :disabled="props.saving"
      :users="props.users"
      :agents="props.agents"
      :enable-internal-note="false"
      :show-send-button="false"
      :placeholder="props.placeholder"
      @update:model-value="onInput"
    />
    <MarkdownRenderer
      v-else-if="props.content.trim()"
      :content="props.content"
      class="text-sm text-[var(--text-secondary)]"
    />
    <p v-else class="text-sm text-[var(--text-muted)]">{{ props.emptyText }}</p>
    <div v-if="props.editing" class="flex justify-end gap-2 mt-2">
      <Button variant="outline" size="sm" :disabled="props.saving" @click="emit('cancel-edit')">
        Cancel
      </Button>
      <Button size="sm" :disabled="props.saving" @click="emit('save')">
        <Save class="h-3.5 w-3.5" />
        {{ props.saving ? "Saving..." : "Save" }}
      </Button>
    </div>
  </Card>
</template>
