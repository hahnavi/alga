<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { Users } from "@lucide/vue";
import { api, type TeamRecord } from "@/lib/api";
import Input from "@/components/ui/Input.vue";
import Modal from "@/components/ui/Modal.vue";
import Card from "@/components/ui/Card.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import { useSearchDebounce } from "@/composables/useSearchDebounce";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useListPage } from "@/composables/useListPage";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";

defineOptions({ name: "TeamsPage" });

const router = useRouter();

const searchInput = ref("");
const showCreateModal = ref(false);
const { submitting: creating, withSubmit: withCreate } = useFormSubmit();

const newName = ref("");
const newDescription = ref("");

const {
  items: teams,
  loading,
  error,
  reload: loadTeams,
} = useListPage<TeamRecord>({
  fetch: () => {
    const params: { q?: string } = {};
    if (searchInput.value.trim()) params.q = searchInput.value.trim();
    return api.getTeams(params);
  },
  entityName: "teams",
});

const { canWrite } = useEntityPermissions("oncall");
const { scheduleSearchReload } = useSearchDebounce(() => loadTeams(), 400);

usePageHeaderActions({
  title: "Teams",
  titleIcon: Users,
  searchInput,
  searchPlaceholder: "Search teams...",
  onSearchInput: scheduleSearchReload,
  showFilters: false,
  showAdd: canWrite,
  onAdd: () => {
    showCreateModal.value = true;
  },
  addLabel: "New Team",
});

onMounted(() => {
  loadTeams();
});

async function handleCreate() {
  if (!newName.value.trim()) return;
  await withCreate(async () => {
    const team = await api.createTeam({
      name: newName.value.trim(),
      description: newDescription.value.trim() || undefined,
    });
    showCreateModal.value = false;
    newName.value = "";
    newDescription.value = "";
    router.push(`/teams/${team.id}`);
  });
}

function goToTeam(id: string) {
  router.push(`/teams/${id}`);
}
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading && teams.length === 0" centered />
    <EmptyState v-else-if="teams.length === 0" message="No teams found.">
      <template #icon>
        <Users class="mb-2 h-6 w-6 opacity-40" />
      </template>
    </EmptyState>
    <div v-else class="space-y-3">
      <Card
        v-for="team in teams"
        :key="team.id"
        class="cursor-pointer transition-all duration-150 hover:border-[var(--border-secondary)] hover:shadow-md"
        role="button"
        tabindex="0"
        @click="goToTeam(team.id)"
        @keydown.enter="goToTeam(team.id)"
      >
        <div class="flex items-center justify-between">
          <div class="min-w-0">
            <h3 class="text-sm font-medium text-[var(--text-primary)]">{{ team.name }}</h3>
            <p v-if="team.description" class="mt-1 text-xs text-[var(--text-muted)] line-clamp-2">
              {{ team.description }}
            </p>
          </div>
          <span class="shrink-0 text-xs text-[var(--text-muted)]">
            {{ team.members?.length ?? 0 }} member{{ (team.members?.length ?? 0) !== 1 ? "s" : "" }}
          </span>
        </div>
      </Card>
    </div>

    <Modal
      :open="showCreateModal"
      title="New Team"
      :loading="creating"
      confirm-label="Create"
      @update:open="showCreateModal = $event"
      @confirm="handleCreate"
    >
      <div class="space-y-4">
        <div>
          <FormLabel for="team-name">Name</FormLabel>
          <Input id="team-name" v-model="newName" placeholder="Team name" required />
        </div>
        <div>
          <FormLabel for="team-description">Description</FormLabel>
          <Input
            id="team-description"
            v-model="newDescription"
            placeholder="Optional description"
          />
        </div>
      </div>
    </Modal>
  </section>
</template>
