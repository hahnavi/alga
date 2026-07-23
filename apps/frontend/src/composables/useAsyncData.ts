import { ref } from "vue";
import { useAsyncOp } from "@/composables/useAsyncOp";

export function useAsyncData<T>(fetcher: () => Promise<T>, errorMsg = "Failed to load") {
  const data = ref<T | null>(null);
  const op = useAsyncOp({ fallbackError: errorMsg });

  async function reload() {
    const result = await op.run(fetcher);
    if (result !== null) data.value = result;
    return result;
  }

  return { data, loading: op.loading, error: op.error, reload };
}
