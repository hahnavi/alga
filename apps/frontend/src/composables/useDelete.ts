import { ref } from "vue";
import { useAsyncOp } from "@/composables/useAsyncOp";

export function useDelete<T>(deleteFn: (item: T) => Promise<unknown>, entityName = "Item") {
  const op = useAsyncOp({ fallbackError: `Failed to delete ${entityName.toLowerCase()}` });
  const deleteTarget = ref<T | null>(null);
  const showDeleteConfirm = ref(false);

  function confirmDelete(item: T) {
    deleteTarget.value = item;
    showDeleteConfirm.value = true;
  }

  async function doDelete() {
    if (!deleteTarget.value) return;
    await op.run(() => deleteFn(deleteTarget.value as T), `${entityName} deleted`);
    showDeleteConfirm.value = false;
    deleteTarget.value = null;
  }

  return {
    deleteTarget,
    showDeleteConfirm,
    deleting: op.loading,
    confirmDelete,
    doDelete,
  };
}
