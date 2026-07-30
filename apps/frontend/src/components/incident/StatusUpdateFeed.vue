<script setup lang="ts">
import { ref } from "vue";
import { Plus } from "@lucide/vue";
import type { IncidentCoordinationMessage, StatusUpdateStatusLevel } from "@/lib/api";
import { api } from "@/lib/api";
import { useToast } from "@/lib/toast";
import { useFormSubmit } from "@/composables/useFormSubmit";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Modal from "@/components/ui/Modal.vue";
import Select from "@/components/ui/Select.vue";
import MarkdownEditor from "@/components/ui/MarkdownEditor.vue";
import MarkdownRenderer from "@/components/ui/MarkdownRenderer.vue";
import SkeletonRows from "@/components/ui/SkeletonRows.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import { CARD_ICON_BTN_CLASS } from "@/lib/uiClasses";
import EmptyState from "@/components/ui/EmptyState.vue";
import { formatTimeAgo } from "@/lib/time";
import { useEscapeKey } from "@/composables/useEscapeKey";

type Props = {
  incidentId: string;
  updates: IncidentCoordinationMessage[];
  canPost: boolean;
  loading: boolean;
  error: string | null;
  incidentStatus?: string;
};

const props = defineProps<Props>();
const emit = defineEmits<{
  posted: [];
  retry: [];
}>();

const { push } = useToast();
const showCompose = ref(false);
const { submitting, withSubmit } = useFormSubmit();
const composeLevel = ref<StatusUpdateStatusLevel>("investigating");
const composeBody = ref("");

useEscapeKey(
  () => {
    showCompose.value = false;
  },
  () => showCompose.value,
);

function defaultLevelForStatus(status?: string): StatusUpdateStatusLevel {
  switch (status) {
    case "detected":
    case "triaging":
      return "investigating";
    case "active":
      return "identified";
    case "mitigated":
      return "monitoring";
    case "resolved":
    case "closed":
      return "resolved";
    default:
      return "investigating";
  }
}

function openCompose() {
  composeLevel.value = defaultLevelForStatus(props.incidentStatus);
  composeBody.value = "";
  showCompose.value = true;
}

async function submitUpdate() {
  if (submitting.value) return;
  const body = composeBody.value.trim();
  if (!body) {
    push("Body is required", "error");
    return;
  }
  await withSubmit(async () => {
    await api.createIncidentStatusUpdate(props.incidentId, {
      status_level: composeLevel.value,
      body,
    });
    showCompose.value = false;
    composeBody.value = "";
    emit("posted");
  }, "Status update posted");
}

function statusLevelFromMessage(msg: IncidentCoordinationMessage): string {
  return (msg.metadata?.status_level as string) ?? "investigating";
}

function statusDotClass(level: string): string {
  switch (level) {
    case "investigating":
      return "bg-amber-500";
    case "identified":
      return "bg-blue-500";
    case "mitigated":
      return "bg-teal-500";
    case "monitoring":
      return "bg-emerald-500";
    case "resolved":
      return "bg-green-500";
    default:
      return "bg-gray-400";
  }
}

function statusLabelClass(level: string): string {
  switch (level) {
    case "investigating":
      return "text-amber-600 dark:text-amber-400";
    case "identified":
      return "text-blue-600 dark:text-blue-400";
    case "mitigated":
      return "text-teal-600 dark:text-teal-400";
    case "monitoring":
      return "text-emerald-600 dark:text-emerald-400";
    case "resolved":
      return "text-green-600 dark:text-green-400";
    default:
      return "text-gray-600 dark:text-gray-400";
  }
}

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
</script>

<template>
  <Card>
    <div class="mb-3 flex items-center justify-between gap-2">
      <h3 class="text-sm font-medium text-[var(--text-primary)]">Status Updates</h3>
      <button
        v-if="canPost"
        type="button"
        :class="CARD_ICON_BTN_CLASS"
        title="Post status update"
        @click="openCompose"
      >
        <Plus class="h-3.5 w-3.5" />
      </button>
    </div>

    <div v-if="error">
      <ErrorBanner :message="error" />
      <Button class="mt-2" variant="link" size="sm" @click="emit('retry')">Retry</Button>
    </div>
    <SkeletonRows v-else-if="loading" :count="3" />
    <EmptyState v-else-if="updates.length === 0" message="No status updates posted yet." />
    <div v-else class="space-y-4">
      <div v-for="(update, index) in updates" :key="update.id" class="flex gap-3">
        <div class="mt-1.5 flex flex-col items-center">
          <span
            class="inline-block h-2.5 w-2.5 rounded-full"
            :class="statusDotClass(statusLevelFromMessage(update))"
          />
          <div
            v-if="index < updates.length - 1"
            class="mt-1 w-px flex-1 bg-[var(--border-primary)]"
          />
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2 text-sm">
            <span class="font-medium" :class="statusLabelClass(statusLevelFromMessage(update))">
              {{ capitalize(statusLevelFromMessage(update)) }}
            </span>
            <span class="text-[var(--text-tertiary)]">&mdash;</span>
            <span class="text-[var(--text-tertiary)]">
              {{ formatTimeAgo(update.created_at) }}
            </span>
            <span v-if="update.actor_display_name" class="text-[var(--text-tertiary)]">
              by {{ update.actor_display_name }}
            </span>
          </div>
          <div class="mt-1 text-sm text-[var(--text-secondary)]">
            <MarkdownRenderer :content="update.body" />
          </div>
        </div>
      </div>
    </div>
  </Card>

  <Modal
    :open="showCompose"
    title="Post Status Update"
    :show-footer="false"
    @close="showCompose = false"
  >
    <div class="space-y-4">
      <div>
        <FormLabel for="status-update-level">Status Level</FormLabel>
        <Select id="status-update-level" v-model="composeLevel">
          <option value="investigating">Investigating</option>
          <option value="identified">Identified</option>
          <option value="monitoring">Monitoring</option>
          <option value="resolved">Resolved</option>
        </Select>
      </div>
      <div>
        <FormLabel for="status-update-body">Message</FormLabel>
        <MarkdownEditor
          id="status-update-body"
          v-model="composeBody"
          placeholder="Describe the current status..."
          :enable-internal-note="false"
        />
      </div>
      <div class="flex justify-end gap-2">
        <Button variant="outline" :disabled="submitting" @click="showCompose = false">
          Cancel
        </Button>
        <Button
          variant="primary"
          :loading="submitting"
          :disabled="!composeBody.trim()"
          @click="submitUpdate"
        >
          Post Update
        </Button>
      </div>
    </div>
  </Modal>
</template>
