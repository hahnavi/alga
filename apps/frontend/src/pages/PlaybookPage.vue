<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { FileText } from "@lucide/vue";
import { api, type PlaybookRecord } from "@/lib/api";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import Select from "@/components/ui/Select.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import PlaybookCard from "@/components/playbook/PlaybookCard.vue";
import PlaybookFormModal from "@/components/playbook/PlaybookFormModal.vue";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useListPage } from "@/composables/useListPage";

defineOptions({ name: "PlaybookPage" });

const router = useRouter();

const kindFilter = ref("");
const searchQuery = ref("");
const showCreateModal = ref(false);

const { canWrite } = useEntityPermissions("playbooks");

const {
  items: playbooks,
  loading,
  error,
  reload: loadPlaybooks,
} = useListPage<PlaybookRecord>({
  fetch: () =>
    api.listPlaybooks({
      kind: kindFilter.value || undefined,
      search: searchQuery.value || undefined,
    }),
  entityName: "playbooks",
});

function viewPlaybook(id: string) {
  router.push(`/playbooks/${id}`);
}

const { showSearch } = usePageHeaderActions({
  title: "Playbooks",
  titleIcon: FileText,
  searchInput: searchQuery,
  searchPlaceholder: "Search playbooks...",
  onSearchInput: loadPlaybooks,
  showAdd: canWrite,
  onAdd: () => {
    showCreateModal.value = true;
  },
  addLabel: "Create Playbook",
});

onMounted(() => {
  loadPlaybooks();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <div v-if="!showSearch" class="flex items-end gap-2">
      <div class="flex flex-col gap-1">
        <FormLabel>Kind</FormLabel>
        <Select v-model="kindFilter" class="min-w-32" @change="loadPlaybooks">
          <option value="">All kinds</option>
          <option value="procedure">Procedure</option>
          <option value="mitigation">Mitigation</option>
        </Select>
      </div>
    </div>

    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading" centered label="Loading playbooks..." />

    <EmptyState v-if="!loading && playbooks.length === 0" message="No playbooks found.">
      <template #icon>
        <FileText class="mb-2 h-6 w-6 opacity-40" />
      </template>
    </EmptyState>

    <div v-if="!loading && playbooks.length > 0" class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      <PlaybookCard v-for="p in playbooks" :key="p.id" :playbook="p" @click="viewPlaybook(p.id)" />
    </div>

    <PlaybookFormModal
      :show="showCreateModal"
      :playbook="null"
      @close="showCreateModal = false"
      @saved="
        showCreateModal = false;
        loadPlaybooks();
      "
    />
  </section>
</template>
