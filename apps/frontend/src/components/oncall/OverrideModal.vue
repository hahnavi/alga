<script setup lang="ts">
import { ref } from "vue";
import Modal from "@/components/ui/Modal.vue";
import UserLabel from "@/components/ui/UserLabel.vue";
import Select from "@/components/ui/Select.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import TimezoneSelect from "@/components/ui/TimezoneSelect.vue";
import DateTimePicker from "@/components/ui/DateTimePicker.vue";
import type { UserInfo } from "@/lib/api";
import { tzLocalToRFC3339 } from "@/lib/time";
defineOptions({ name: "OverrideModal" });

const props = defineProps<{
  open: boolean;
  loading?: boolean;
  users?: UserInfo[];
  scheduleId: string;
  initialTimezone?: string;
}>();

const emit = defineEmits<{
  "update:open": [value: boolean];
  submit: [payload: { user_id: string; start_at: string; end_at: string }];
}>();

const selectedUserId = ref("");
const startAt = ref("");
const endAt = ref("");
const timezone = ref(props.initialTimezone || "UTC");

function handleSubmit() {
  if (!selectedUserId.value || !startAt.value || !endAt.value) return;
  if (new Date(startAt.value) >= new Date(endAt.value)) return;
  const startAtRFC = tzLocalToRFC3339(startAt.value, timezone.value);
  const endAtRFC = tzLocalToRFC3339(endAt.value, timezone.value);
  if (!startAtRFC || !endAtRFC) return;
  emit("submit", {
    user_id: selectedUserId.value,
    start_at: startAtRFC,
    end_at: endAtRFC,
  });
}

function resetAndClose() {
  selectedUserId.value = "";
  startAt.value = "";
  endAt.value = "";
  timezone.value = props.initialTimezone || "UTC";
  emit("update:open", false);
}
</script>

<template>
  <Modal
    :open="open"
    title="Create Override"
    :loading="loading"
    confirm-label="Create"
    @update:open="resetAndClose"
    @confirm="handleSubmit"
  >
    <div class="space-y-4">
      <div>
        <FormLabel for="override-user">User</FormLabel>
        <Select
          id="override-user"
          :model-value="selectedUserId"
          class="w-full"
          @update:model-value="selectedUserId = $event"
        >
          <option value="" disabled>Select a user</option>
          <option v-for="u in users" :key="u.id" :value="u.id">
            <UserLabel :user="u" />
          </option>
        </Select>
      </div>
      <div>
        <FormLabel for="override-tz">Timezone</FormLabel>
        <TimezoneSelect id="override-tz" v-model="timezone" />
      </div>
      <div>
        <FormLabel for="override-start">Start</FormLabel>
        <DateTimePicker id="override-start" v-model="startAt" placeholder="Start date & time" />
      </div>
      <div>
        <FormLabel for="override-end">End</FormLabel>
        <DateTimePicker id="override-end" v-model="endAt" placeholder="End date & time" />
      </div>
    </div>
  </Modal>
</template>
