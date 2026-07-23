<script setup lang="ts">
import { ref, watch } from "vue";
import { Plus, Trash2 } from "@lucide/vue";
import Input from "@/components/ui/Input.vue";
import NumberInput from "@/components/ui/NumberInput.vue";
import Select from "@/components/ui/Select.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import UserLabel from "@/components/ui/UserLabel.vue";
import Button from "@/components/ui/Button.vue";
import type { UserInfo, TeamRecord } from "@/lib/api";

defineOptions({ name: "EscalationPolicyForm" });

export type EscalationPolicyTargetInput = {
  target_type: string;
  target_user_id?: string;
  target_team_id?: string;
};

export type EscalationPolicyLevelInput = {
  delay_minutes: number;
  notify_channels: string[];
  targets: EscalationPolicyTargetInput[];
};

export type EscalationPolicyFormState = {
  name: string;
  description: string;
  repeat: number;
  levels: EscalationPolicyLevelInput[];
};

const props = withDefaults(
  defineProps<{
    modelValue: EscalationPolicyFormState;
    disabled?: boolean;
    users: UserInfo[];
    teams: TeamRecord[];
    idPrefix?: string;
  }>(),
  {
    disabled: false,
    idPrefix: "pol",
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: EscalationPolicyFormState];
}>();

// Two-way binding via a deep-cloned local ref, with a `syncingFromProps`
// guard to break the emit→prop-change→emit cycle: when the prop changes
// because we just emitted, we re-clone into `local` but don't fire the
// `local` watcher. When the user edits `local`, we emit and set
// `syncingFromProps` so the subsequent prop change is ignored by the
// prop watcher. Replaces the original syncingFromProps / suppressEmit
// two-flag dance; one flag is enough because the directions are
// mutually exclusive (a prop change can only be ours OR the parent's).
const cloneState = (state: EscalationPolicyFormState): EscalationPolicyFormState =>
  JSON.parse(JSON.stringify(state));
const local = ref<EscalationPolicyFormState>(cloneState(props.modelValue));
let syncingFromProps = false;

watch(
  () => props.modelValue,
  (next) => {
    if (syncingFromProps) {
      syncingFromProps = false;
      return;
    }
    local.value = cloneState(next);
  },
  { deep: true },
);

watch(
  local,
  (next) => {
    syncingFromProps = true;
    emit("update:modelValue", cloneState(next));
  },
  { deep: true },
);

function addLevel() {
  local.value.levels.push({
    delay_minutes: 5,
    notify_channels: [],
    targets: [{ target_type: "user" }],
  });
}

function removeLevel(idx: number) {
  local.value.levels.splice(idx, 1);
}

function addTarget(level: EscalationPolicyLevelInput) {
  level.targets.push({ target_type: "user" });
}

function removeTarget(level: EscalationPolicyLevelInput, idx: number) {
  level.targets.splice(idx, 1);
}
</script>

<template>
  <div class="space-y-4">
    <div>
      <FormLabel :for="`${idPrefix}-name`">Name</FormLabel>
      <Input :id="`${idPrefix}-name`" v-model="local.name" placeholder="Policy name" required />
    </div>
    <div>
      <FormLabel :for="`${idPrefix}-desc`">Description</FormLabel>
      <Input
        :id="`${idPrefix}-desc`"
        v-model="local.description"
        placeholder="Optional description"
      />
    </div>
    <div>
      <FormLabel :for="`${idPrefix}-repeat`">Repeat Count</FormLabel>
      <NumberInput :id="`${idPrefix}-repeat`" v-model.number="local.repeat" placeholder="0" />
    </div>
    <div class="space-y-3">
      <div class="flex items-center justify-between">
        <span class="text-sm font-medium text-[var(--text-primary)]">Levels</span>
        <Button variant="outline" size="sm" @click="addLevel">
          <Plus class="h-3 w-3" />
          Level
        </Button>
      </div>
      <div
        v-for="(level, li) in local.levels"
        :key="li"
        class="space-y-2 rounded-md border border-[var(--border-primary)] p-3"
      >
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-[var(--text-muted)]"> Level {{ li + 1 }} </span>
          <Button variant="outline" size="sm" @click="removeLevel(li)">
            <Trash2 class="h-3 w-3" />
          </Button>
        </div>
        <div>
          <FormLabel>Delay (minutes)</FormLabel>
          <NumberInput v-model.number="level.delay_minutes" />
        </div>
        <div class="space-y-1">
          <div class="flex items-center justify-between">
            <span class="text-xs text-[var(--text-muted)]">Targets</span>
            <Button variant="outline" size="sm" @click="addTarget(level)">
              <Plus class="h-3 w-3" />
            </Button>
          </div>
          <div v-for="(target, ti) in level.targets" :key="ti" class="flex items-center gap-2">
            <Select
              :model-value="target.target_type"
              class="flex-1"
              @update:model-value="target.target_type = $event"
            >
              <option value="user">User</option>
              <option value="team">Team</option>
            </Select>
            <Select
              v-if="target.target_type === 'user'"
              :model-value="target.target_user_id"
              class="flex-1"
              @update:model-value="target.target_user_id = $event"
            >
              <option value="">Select user</option>
              <option v-for="u in users" :key="u.id" :value="u.id">
                <UserLabel :user="u" />
              </option>
            </Select>
            <Select
              v-if="target.target_type === 'team'"
              :model-value="target.target_team_id"
              class="flex-1"
              @update:model-value="target.target_team_id = $event"
            >
              <option value="">Select team</option>
              <option v-for="t in teams" :key="t.id" :value="t.id">{{ t.name }}</option>
            </Select>
            <Button variant="outline" size="sm" @click="removeTarget(level, ti)">
              <Trash2 class="h-3 w-3" />
            </Button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
