<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { computed, ref } from "vue";
import { UserPlus, Shield, Bot } from "@lucide/vue";
import { api, type AgentCapability, type ICSRoleRecord, type AgentTokenRow } from "@/lib/api";
import ICSRoleCard from "@/components/incident/ICSRoleCard.vue";
import ICSSlotCard from "@/components/incident/ICSSlotCard.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import IconBtn from "@/components/ui/IconBtn.vue";
import Input from "@/components/ui/Input.vue";
import Modal from "@/components/ui/Modal.vue";
import Select from "@/components/ui/Select.vue";
import UserLabel from "@/components/ui/UserLabel.vue";
import { useToast } from "@/lib/toast";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useUsersIfPermitted } from "@/composables/useUsers";

const props = defineProps<{
  roles: ICSRoleRecord[];
  incidentId: string;
}>();

const emit = defineEmits<{
  reloadRoles: [];
}>();

const { push } = useToast();
const { canCommand } = useEntityPermissions("incidents");

const showAssignDialog = ref(false);
const replacingRole = ref<ICSRoleRecord | null>(null);

type AssignableRoleType = Extract<
  ICSRoleRecord["role_type"],
  "incident_commander" | "communications_lead" | "responder"
>;

const assignRoleType = ref<AssignableRoleType | "">("");
const assignUserId = ref("");
const assignScope = ref("");
const assignSubmitting = ref(false);
const assignError = ref("");
const { users: allUsers, loadUsers } = useUsersIfPermitted("users:read");
const allAgents = ref<AgentTokenRow[]>([]);
const assignAgentTokenId = ref("");
const assignMode = ref<"user" | "agent">("user");

const currentIC = computed(() =>
  props.roles.find((r) => r.role_type === "incident_commander" && r.status === "active"),
);

const currentCommunicator = computed(() =>
  props.roles.find((r) => r.role_type === "communications_lead" && r.status === "active"),
);

const responders = computed(() =>
  props.roles.filter((r) => r.status === "active" && r.role_type === "responder"),
);

const hasActiveRoles = computed(
  () =>
    Boolean(currentIC.value) || Boolean(currentCommunicator.value) || responders.value.length > 0,
);

const roleOptions = [
  { value: "incident_commander", label: "Incident Commander" },
  { value: "communications_lead", label: "Communicator" },
  { value: "responder", label: "Responder" },
] as const satisfies readonly { value: AssignableRoleType; label: string }[];

const roleCapability: Record<AssignableRoleType, AgentCapability> = {
  incident_commander: "command",
  communications_lead: "communicate",
  responder: "investigate",
};

const availableAgents = computed(() => {
  if (!assignRoleType.value) return [];
  const required = roleCapability[assignRoleType.value];
  return allAgents.value.filter((a) => a.capabilities?.includes(required));
});

function getRoleLabel(type: string): string {
  const opt = roleOptions.find((o) => o.value === type);
  return opt ? opt.label : type;
}

async function openAssignDialog(roleType?: AssignableRoleType) {
  replacingRole.value = null;
  assignRoleType.value = roleType ?? "responder";
  assignUserId.value = "";
  assignAgentTokenId.value = "";
  assignMode.value = "user";
  assignScope.value = "";
  assignError.value = "";
  assignSubmitting.value = false;
  try {
    const [, tokens] = await Promise.all([loadUsers(), api.getAgentTokens()]);
    allAgents.value = tokens.filter((t) => t.enabled);
  } catch {
    allAgents.value = [];
  }
  showAssignDialog.value = true;
}

function closeAssignDialog() {
  showAssignDialog.value = false;
  replacingRole.value = null;
}

async function submitAssignRole() {
  if (assignSubmitting.value) return;
  const uid = assignUserId.value.trim();
  const atid = assignAgentTokenId.value.trim();
  if (assignMode.value === "user" && !uid) {
    assignError.value = "User is required.";
    return;
  }
  if (assignMode.value === "agent" && !atid) {
    assignError.value = "Agent is required.";
    return;
  }
  if (!assignRoleType.value) {
    assignError.value = "Role type is required.";
    return;
  }
  assignSubmitting.value = true;
  assignError.value = "";
  try {
    const body: Parameters<typeof api.assignICSRole>[1] = {
      role_type: assignRoleType.value,
      scope_description: assignScope.value.trim() || undefined,
    };
    if (assignMode.value === "agent") {
      body.agent_token_id = atid;
    } else {
      body.user_id = uid;
    }

    if (replacingRole.value) {
      await api.endICSRole(props.incidentId, replacingRole.value.id, { ended_reason: "replaced" });
    }

    await api.assignICSRole(props.incidentId, body);
    showAssignDialog.value = false;
    const msg = replacingRole.value ? "ICS role replaced" : "ICS role assigned";
    replacingRole.value = null;
    push(msg, "success");
    emit("reloadRoles");
  } catch (err) {
    assignError.value = getErrorMessage(err, "Failed to assign role");
  } finally {
    assignSubmitting.value = false;
  }
}

async function endRole(roleId: string) {
  try {
    await api.endICSRole(props.incidentId, roleId, { ended_reason: "replaced" });
    push("Role ended", "success");
    emit("reloadRoles");
  } catch (err) {
    push(getErrorMessage(err, "Failed to end role"), "error");
  }
}

async function replaceRole(role: ICSRoleRecord | undefined | null) {
  if (!role) return;
  replacingRole.value = role;
  assignRoleType.value = role.role_type as AssignableRoleType;
  assignUserId.value = "";
  assignAgentTokenId.value = "";
  assignMode.value = "user";
  assignScope.value = role.scope_description || "";
  assignError.value = "";
  assignSubmitting.value = false;
  try {
    const [, tokens] = await Promise.all([loadUsers(), api.getAgentTokens()]);
    allAgents.value = tokens.filter((t) => t.enabled);
  } catch {
    allAgents.value = [];
  }
  showAssignDialog.value = true;
}
</script>

<template>
  <Card>
    <div class="mb-3 flex items-center gap-2">
      <h3 class="flex items-center gap-2 text-sm font-medium text-[var(--text-primary)]">
        <Shield class="h-4 w-4" />
        ICS Command Structure
      </h3>
    </div>

    <div class="space-y-3">
      <ICSSlotCard
        role-type="incident_commander"
        :role="currentIC ?? null"
        :can-manage="canCommand"
        responsibility-hint="Owns decisions, escalation, and final say on the incident response."
        @assign="openAssignDialog('incident_commander')"
        @replace="replaceRole(currentIC)"
        @end="endRole"
      />

      <ICSSlotCard
        role-type="communications_lead"
        :role="currentCommunicator ?? null"
        :can-manage="canCommand"
        responsibility-hint="Owns stakeholder messaging, status updates, and external comms cadence."
        @assign="openAssignDialog('communications_lead')"
        @replace="replaceRole(currentCommunicator)"
        @end="endRole"
      />

      <div v-if="responders.length > 0 || canCommand" class="space-y-2">
        <div class="flex items-center justify-between">
          <p class="text-xs font-semibold uppercase tracking-wide text-[var(--text-muted)]">
            Responders
          </p>
          <IconBtn
            v-if="canCommand"
            :icon="UserPlus"
            label="Add responder"
            size="sm"
            @click="() => openAssignDialog('responder')"
          />
        </div>
        <ICSRoleCard v-for="role in responders" :key="role.id" :role="role" @end-role="endRole" />
      </div>

      <div v-if="!hasActiveRoles" class="py-4 text-center text-xs text-[var(--text-muted)]">
        No active ICS roles assigned
      </div>
    </div>

    <!-- Assign ICS Role dialog -->
    <Modal
      :open="showAssignDialog"
      :title="
        replacingRole ? `Replace ${getRoleLabel(replacingRole.role_type)}` : 'Assign ICS Role'
      "
      max-width="lg"
      :prevent-close="assignSubmitting"
      @update:open="!$event && closeAssignDialog()"
      @close="closeAssignDialog"
    >
      <form class="space-y-4" @submit.prevent="submitAssignRole">
        <ErrorBanner :message="assignError" />
        <div class="flex gap-2">
          <button
            type="button"
            :class="[
              'flex-1 rounded-md px-3 py-2 text-sm font-medium transition-colors cursor-pointer',
              assignMode === 'user'
                ? 'bg-[var(--bg-secondary)] text-[var(--text-primary)]'
                : 'text-[var(--text-muted)] hover:bg-[var(--bg-secondary)]',
            ]"
            @click="assignMode = 'user'"
          >
            User
          </button>
          <button
            type="button"
            :class="[
              'flex-1 rounded-md px-3 py-2 text-sm font-medium transition-colors cursor-pointer',
              assignMode === 'agent'
                ? 'bg-[var(--bg-secondary)] text-[var(--text-primary)]'
                : 'text-[var(--text-muted)] hover:bg-[var(--bg-secondary)]',
            ]"
            @click="assignMode = 'agent'"
          >
            <Bot class="inline h-3.5 w-3.5" />
            Agent
          </button>
        </div>
        <div>
          <FormLabel for="assign-ics-role-type" compact required class="mb-1.5 block">
            Role Type
          </FormLabel>
          <Select
            id="assign-ics-role-type"
            v-model="assignRoleType"
            class="w-full"
            :disabled="assignSubmitting || !!replacingRole"
          >
            <option value="">Select a role</option>
            <option v-for="role in roleOptions" :key="role.value" :value="role.value">
              {{ role.label }}
            </option>
          </Select>
        </div>
        <div v-if="assignMode === 'user'">
          <FormLabel for="assign-ics-user" compact required class="mb-1.5 block"> User </FormLabel>
          <Select
            id="assign-ics-user"
            :model-value="assignUserId"
            class="w-full"
            :disabled="assignSubmitting"
            @update:model-value="assignUserId = $event"
          >
            <option value="">Select a user</option>
            <option v-for="u in allUsers" :key="u.id" :value="u.id">
              <UserLabel :user="u" />
            </option>
          </Select>
        </div>
        <div v-else>
          <FormLabel for="assign-ics-agent" compact required class="mb-1.5 block">
            Agent
          </FormLabel>
          <Select
            id="assign-ics-agent"
            :model-value="assignAgentTokenId"
            class="w-full"
            :disabled="assignSubmitting"
            @update:model-value="assignAgentTokenId = $event"
          >
            <option value="">Select an agent</option>
            <option v-for="a in availableAgents" :key="a.id" :value="a.id">
              {{ a.name }} ({{ a.agent_type }})
            </option>
          </Select>
        </div>
        <div>
          <FormLabel for="assign-ics-scope" compact class="mb-1.5 block">
            Scope Description
          </FormLabel>
          <Input
            id="assign-ics-scope"
            v-model="assignScope"
            placeholder="e.g. Database reliability, Customer communications"
            :disabled="assignSubmitting"
          />
        </div>
      </form>
      <template #footer>
        <Button variant="outline" :disabled="assignSubmitting" @click="closeAssignDialog">
          Cancel
        </Button>
        <Button :disabled="assignSubmitting" @click="submitAssignRole">
          {{
            assignSubmitting
              ? replacingRole
                ? "Replacing…"
                : "Assigning…"
              : replacingRole
                ? "Replace"
                : "Assign"
          }}
        </Button>
      </template>
    </Modal>
  </Card>
</template>
