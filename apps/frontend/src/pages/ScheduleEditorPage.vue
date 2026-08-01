<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { Clock, Trash2, Plus, X, Repeat, UserCog, CalendarRange } from "@lucide/vue";
import { api } from "@/lib/api";
import type {
  OnCallScheduleRecord,
  ScheduleLayerInput,
  ScheduleLayerRecord,
  ScheduleOverrideRecord,
  TeamMemberRecord,
  UserInfo,
} from "@/lib/api";
import { useAuthStore } from "@/stores/auth";
import { useToast } from "@/lib/toast";
import { useAsyncData } from "@/composables/useAsyncData";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useUsersIfPermitted } from "@/composables/useUsers";
import { usePageHeader } from "@/composables/usePageHeader";
import { getErrorMessage } from "@/lib/error";
import { formatTime } from "@/lib/time";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Select from "@/components/ui/Select.vue";
import NumberInput from "@/components/ui/NumberInput.vue";
import Card from "@/components/ui/Card.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Checkbox from "@/components/ui/Checkbox.vue";
import Switch from "@/components/ui/Switch.vue";
import TimezoneSelect from "@/components/ui/TimezoneSelect.vue";
import UserLabel from "@/components/ui/UserLabel.vue";
import OverrideModal from "@/components/oncall/OverrideModal.vue";
import ScheduleTimeline from "@/components/oncall/ScheduleTimeline.vue";
import Tabs, { type Tab } from "@/components/ui/Tabs.vue";
import DateTimePicker from "@/components/ui/DateTimePicker.vue";
import TimePicker from "@/components/ui/TimePicker.vue";

defineOptions({ name: "ScheduleEditorPage" });

const route = useRoute();
const auth = useAuthStore();
const { push } = useToast();

const scheduleId = computed(() => (route.params.id as string) ?? "");
const { canWrite: canEdit } = useEntityPermissions("oncall");

const schedule = ref<OnCallScheduleRecord | null>(null);
const overrides = ref<ScheduleOverrideRecord[]>([]);
const users = ref<UserInfo[]>([]);
const showOverride = ref(false);
const { users: permittedUsers, loadUsers: loadPermittedUsers } =
  useUsersIfPermitted("users:manage");

type LayerForm = {
  name: string;
  rotation_type: "hourly" | "daily" | "weekly" | "monthly";
  rotation_interval: number;
  start_date: string; // datetime-local value
  end_date: string; // datetime-local value or ""
  timezone: string;
  start_time: string; // HH:MM
  end_time: string; // HH:MM or ""
  use_window: boolean;
  days_of_week: string[];
  use_days: boolean;
  priority: number;
  user_ids: string[];
};

const WEEKDAYS = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"];

function isoToLocalInput(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function blankLayer(n: number): LayerForm {
  return {
    name: `Rotation ${n}`,
    rotation_type: "weekly",
    rotation_interval: 1,
    start_date: isoToLocalInput(new Date().toISOString()),
    end_date: "",
    timezone: "UTC",
    start_time: "00:00",
    end_time: "",
    use_window: false,
    days_of_week: [],
    use_days: false,
    priority: 0,
    user_ids: [],
  };
}

function recordToForm(l: ScheduleLayerRecord): LayerForm {
  const hasDays = (l.days_of_week?.length ?? 0) > 0;
  return {
    name: l.name,
    rotation_type: (l.rotation_type as LayerForm["rotation_type"]) || "weekly",
    rotation_interval: l.rotation_interval || 1,
    start_date: isoToLocalInput(l.start_date),
    end_date: l.end_date ? isoToLocalInput(l.end_date) : "",
    timezone: l.timezone || "UTC",
    start_time: l.start_time || "00:00",
    end_time: l.end_time || "",
    use_window: !!(l.end_time && l.end_time !== "" && l.start_time),
    days_of_week: hasDays ? [...l.days_of_week] : [],
    use_days: hasDays,
    priority: l.priority || 0,
    user_ids: [...(l.user_ids ?? [])],
  };
}

function formToLayer(l: LayerForm): ScheduleLayerInput {
  const out: ScheduleLayerInput = {
    name: l.name,
    rotation_type: l.rotation_type,
    rotation_interval: l.rotation_interval,
    start_date: new Date(l.start_date).toISOString(),
    timezone: l.timezone,
    start_time: l.start_time || "00:00",
    end_time: l.use_window ? l.end_time : "",
    days_of_week: l.use_days ? l.days_of_week : [],
    priority: l.priority,
    user_ids: l.user_ids,
  };
  if (l.end_date) {
    out.end_date = new Date(l.end_date).toISOString();
  }
  return out;
}

const layers = ref<LayerForm[]>([]);

const initialOverrideTimezone = computed(() => {
  const sorted = [...layers.value].sort((a, b) => b.priority - a.priority);
  return sorted[0]?.timezone || schedule.value?.team_name ? "UTC" : "UTC";
});

const { loading, error, reload } = useAsyncData(async () => {
  const sched = await api.getSchedule(scheduleId.value);
  const [overs, members] = await Promise.all([
    api.getScheduleOverrides(scheduleId.value),
    sched.team_id ? api.getTeamMembers(sched.team_id) : Promise.resolve<TeamMemberRecord[]>([]),
  ]);
  const memberUserIds = new Set(members.map((m) => m.user_id));
  await loadPermittedUsers();
  let resolvedUsers: UserInfo[];
  if (auth.hasPermission("users:manage")) {
    resolvedUsers = permittedUsers.value;
  } else {
    resolvedUsers = members.map<UserInfo>((m) => ({
      id: m.user_id,
      email: m.user_email ?? "",
      full_name: m.user_name,
      role: "member",
      created_at: "",
    }));
  }
  schedule.value = sched;
  overrides.value = overs;
  users.value = resolvedUsers.filter((u) => memberUserIds.has(u.id));
  layers.value = (sched.layers ?? []).map(recordToForm);
  return sched;
});

usePageHeader(() => {
  const sched = schedule.value;
  if (!sched) return null;
  return {
    title: sched.team_name || "On-Call",
    options: {
      titleIcon: h(Clock, { class: "h-5 w-5 text-[var(--text-muted)]" }),
    },
  };
});

onMounted(() => {
  if (scheduleId.value) void reload();
});
watch(scheduleId, (id) => {
  if (id) void reload();
});

const { submitting, withSubmit } = useFormSubmit();

async function saveLayers() {
  if (!schedule.value) return;
  await withSubmit(async () => {
    const updated = await api.updateSchedule(schedule.value!.id, {
      layers: layers.value.map(formToLayer),
    });
    schedule.value = updated;
    layers.value = (updated.layers ?? []).map(recordToForm);
  }, "Rotations saved");
}

function addLayer() {
  layers.value.push(blankLayer(layers.value.length + 1));
}
function removeLayer(i: number) {
  if (layers.value.length <= 1) return;
  layers.value.splice(i, 1);
}

function toggleDay(layer: LayerForm, day: string) {
  const idx = layer.days_of_week.indexOf(day);
  if (idx === -1) {
    layer.days_of_week.push(day);
  } else {
    layer.days_of_week.splice(idx, 1);
  }
}

function availableUsers(layer: LayerForm): UserInfo[] {
  return users.value.filter((u) => !layer.user_ids.includes(u.id));
}
function addUser(layer: LayerForm, userId: string) {
  if (userId && !layer.user_ids.includes(userId)) {
    layer.user_ids.push(userId);
  }
}
function removeUser(layer: LayerForm, userId: string) {
  layer.user_ids = layer.user_ids.filter((id) => id !== userId);
}

async function createOverride(input: { user_id: string; start_at: string; end_at: string }) {
  await withSubmit(async () => {
    await api.createScheduleOverride(scheduleId.value, input);
    overrides.value = await api.getScheduleOverrides(scheduleId.value);
    showOverride.value = false;
  }, "Override added");
}

async function deleteOverride(id: string) {
  try {
    await api.deleteScheduleOverride(id);
    overrides.value = overrides.value.filter((o) => o.id !== id);
    push("Override removed", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to remove override"), "error");
  }
}

type TabId = "timeline" | "rotations" | "overrides";
const activeTab = ref<TabId>("timeline");

const tabItems: Tab<TabId>[] = [
  { id: "timeline", label: "Schedule", icon: CalendarRange },
  { id: "rotations", label: "Rotations", icon: Repeat },
  { id: "overrides", label: "Overrides", icon: UserCog },
];

function userById(id: string): UserInfo | undefined {
  return users.value.find((u) => u.id === id);
}
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading" centered />

    <template v-if="schedule && !loading">
      <Tabs
        v-model="activeTab"
        :tabs="tabItems"
        aria-label="Schedule sections"
        id-prefix="schedule"
      >
        <!-- Schedule (unified timeline) -->
        <template #panel-timeline>
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <p class="text-sm text-[var(--text-muted)]">
                Rotations, overrides, and final on-call resolved over time.
              </p>
              <Button v-if="canEdit" size="sm" @click="showOverride = true">
                <Plus class="h-3.5 w-3.5" /> Add override
              </Button>
            </div>
            <ScheduleTimeline :schedule-id="scheduleId" :users="users" :can-edit="canEdit" />
          </div>
        </template>

        <!-- Rotations -->
        <template #panel-rotations>
          <div class="space-y-3">
            <EmptyState
              v-if="layers.length === 0"
              message="No rotations yet. Add one to start scheduling."
            >
              <template #footer>
                <Button v-if="canEdit" size="sm" @click="addLayer"
                  ><Plus class="h-3.5 w-3.5" /> Add rotation</Button
                >
              </template>
            </EmptyState>

            <Card v-for="(layer, i) in layers" :key="i" class="space-y-3">
              <div class="flex items-start justify-between gap-2">
                <div class="flex-1">
                  <FormLabel>Rotation name</FormLabel>
                  <Input v-if="canEdit" v-model="layer.name" />
                  <p v-else class="text-sm font-medium text-[var(--text-primary)]">
                    {{ layer.name }}
                  </p>
                </div>
                <div class="w-20">
                  <FormLabel>Priority</FormLabel>
                  <NumberInput v-if="canEdit" v-model="layer.priority" :min="0" />
                  <p v-else class="text-sm">{{ layer.priority }}</p>
                </div>
              </div>

              <div>
                <FormLabel>Timezone</FormLabel>
                <TimezoneSelect v-if="canEdit" v-model="layer.timezone" />
                <p v-else class="text-sm">{{ layer.timezone || "UTC" }}</p>
              </div>

              <div class="grid gap-3 sm:grid-cols-2">
                <div>
                  <FormLabel>Rotation type</FormLabel>
                  <Select v-if="canEdit" v-model="layer.rotation_type" class="w-full">
                    <option value="hourly">Hourly</option>
                    <option value="daily">Daily</option>
                    <option value="weekly">Weekly</option>
                    <option value="monthly">Monthly</option>
                  </Select>
                  <p v-else class="text-sm capitalize">{{ layer.rotation_type }}</p>
                </div>
                <div>
                  <FormLabel>Every (interval)</FormLabel>
                  <NumberInput v-if="canEdit" v-model="layer.rotation_interval" :min="1" />
                  <p v-else class="text-sm">{{ layer.rotation_interval }}</p>
                </div>
                <div>
                  <FormLabel>Starts</FormLabel>
                  <DateTimePicker
                    v-if="canEdit"
                    v-model="layer.start_date"
                    placeholder="Pick start date & time"
                  />
                  <p v-else class="text-sm">{{ layer.start_date }}</p>
                </div>
                <div>
                  <FormLabel>Ends (optional)</FormLabel>
                  <DateTimePicker
                    v-if="canEdit"
                    v-model="layer.end_date"
                    placeholder="Pick end date & time"
                  />
                  <p v-else class="text-sm">{{ layer.end_date || "—" }}</p>
                </div>
              </div>

              <!-- Participants -->
              <div>
                <FormLabel>Participants (in rotation order)</FormLabel>
                <div v-if="canEdit" class="space-y-2">
                  <div class="flex flex-wrap gap-1.5">
                    <span
                      v-for="uid in layer.user_ids"
                      :key="uid"
                      class="inline-flex items-center gap-1 rounded-full bg-[var(--bg-secondary)] px-2 py-0.5 text-xs text-[var(--text-secondary)]"
                    >
                      <UserLabel v-if="userById(uid)" :user="userById(uid)!" />
                      <span v-else>{{ uid.slice(0, 8) }}</span>
                      <button
                        class="text-[var(--text-muted)] hover:text-[var(--text-error)]"
                        @click="removeUser(layer, uid)"
                      >
                        <X class="h-3 w-3" />
                      </button>
                    </span>
                    <span
                      v-if="layer.user_ids.length === 0"
                      class="text-xs text-[var(--text-muted)]"
                    >
                      No participants yet
                    </span>
                  </div>
                  <Select
                    :model-value="''"
                    class="w-full sm:w-64"
                    @update:model-value="
                      (v) => {
                        if (v) addUser(layer, v);
                      }
                    "
                  >
                    <option value="" disabled>Add participant…</option>
                    <option v-for="u in availableUsers(layer)" :key="u.id" :value="u.id">
                      <UserLabel :user="u" />
                    </option>
                  </Select>
                </div>
                <div v-else class="flex flex-wrap gap-1.5">
                  <span
                    v-for="uid in layer.user_ids"
                    :key="uid"
                    class="inline-flex items-center rounded-full bg-[var(--bg-secondary)] px-2 py-0.5 text-xs text-[var(--text-secondary)]"
                  >
                    <UserLabel v-if="userById(uid)" :user="userById(uid)!" />
                    <span v-else>{{ uid.slice(0, 8) }}</span>
                  </span>
                </div>
              </div>

              <!-- Daily active window -->
              <div class="rounded-md border border-[var(--border-secondary)] p-2">
                <label
                  class="flex items-center gap-2 text-xs font-medium text-[var(--text-secondary)]"
                >
                  <Switch v-if="canEdit" v-model="layer.use_window" />
                  <span>Limit to specific hours each day</span>
                </label>
                <div v-if="layer.use_window" class="mt-2 grid grid-cols-2 gap-2">
                  <div>
                    <FormLabel>From</FormLabel>
                    <TimePicker v-if="canEdit" v-model="layer.start_time" placeholder="From" />
                    <p v-else class="text-sm">{{ layer.start_time }}</p>
                  </div>
                  <div>
                    <FormLabel>To</FormLabel>
                    <TimePicker v-if="canEdit" v-model="layer.end_time" placeholder="To" />
                    <p v-else class="text-sm">{{ layer.end_time }}</p>
                  </div>
                </div>
              </div>

              <!-- Active weekdays -->
              <div class="rounded-md border border-[var(--border-secondary)] p-2">
                <label
                  class="flex items-center gap-2 text-xs font-medium text-[var(--text-secondary)]"
                >
                  <Switch v-if="canEdit" v-model="layer.use_days" />
                  <span>Limit to specific weekdays</span>
                </label>
                <div v-if="layer.use_days" class="mt-2 flex flex-wrap gap-3">
                  <label
                    v-for="day in WEEKDAYS"
                    :key="day"
                    class="flex cursor-pointer items-center gap-1.5 text-xs text-[var(--text-secondary)]"
                  >
                    <Checkbox
                      v-if="canEdit"
                      :model-value="layer.days_of_week.includes(day)"
                      @update:model-value="toggleDay(layer, day)"
                    />
                    <span>{{ day.slice(0, 3) }}</span>
                  </label>
                </div>
              </div>

              <div v-if="canEdit" class="flex justify-end">
                <Button
                  variant="outline"
                  size="sm"
                  :disabled="layers.length <= 1"
                  @click="removeLayer(i)"
                >
                  <Trash2 class="h-3.5 w-3.5" /> Remove rotation
                </Button>
              </div>
            </Card>

            <div v-if="canEdit" class="flex items-center justify-between">
              <Button variant="outline" size="sm" @click="addLayer"
                ><Plus class="h-3.5 w-3.5" /> Add rotation</Button
              >
              <Button size="sm" :loading="submitting" @click="saveLayers">Save rotations</Button>
            </div>
          </div>
        </template>

        <!-- Overrides -->
        <template #panel-overrides>
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <p class="text-sm text-[var(--text-muted)]">
                Overrides temporarily place a specific user on call, taking precedence over
                rotations.
              </p>
              <Button v-if="canEdit" size="sm" @click="showOverride = true"
                ><Plus class="h-3.5 w-3.5" /> Add override</Button
              >
            </div>
            <div v-if="overrides.length === 0">
              <EmptyState message="No overrides scheduled." />
            </div>
            <Card v-else class="divide-y divide-[var(--border-secondary)] p-0">
              <div
                v-for="o in overrides"
                :key="o.id"
                class="flex items-center justify-between gap-3 px-3 py-2"
              >
                <div class="flex items-center gap-2">
                  <UserLabel v-if="userById(o.user_id)" :user="userById(o.user_id)!" />
                  <span v-else class="text-sm">{{ o.user_id.slice(0, 8) }}</span>
                  <span class="text-xs text-[var(--text-muted)]">
                    {{ formatTime(o.start_at) }} →
                    {{ formatTime(o.end_at) }}
                  </span>
                </div>
                <Button v-if="canEdit" variant="outline" size="sm" @click="deleteOverride(o.id)">
                  <Trash2 class="h-3.5 w-3.5" />
                </Button>
              </div>
            </Card>
          </div>
        </template>
      </Tabs>
    </template>

    <OverrideModal
      :open="showOverride"
      :users="users"
      :schedule-id="scheduleId"
      :initial-timezone="initialOverrideTimezone"
      @update:open="(v) => (showOverride = v)"
      @submit="createOverride"
    />
  </section>
</template>
