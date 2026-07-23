import { ref } from "vue";
import { useAsyncOp } from "@/composables/useAsyncOp";

export function useListPage<T>(opts: {
  fetch: () => Promise<{ items: T[]; total?: number }>;
  entityName: string;
}) {
  const items = ref<T[]>([]);
  const total = ref(0);
  const op = useAsyncOp({
    fallbackError: `Failed to load ${opts.entityName.toLowerCase()}`,
  });

  async function reload() {
    const result = await op.run(opts.fetch);
    if (result) {
      items.value = result.items ?? [];
      total.value = result.total ?? items.value.length;
    }
  }

  return { items, total, loading: op.loading, error: op.error, reload };
}
