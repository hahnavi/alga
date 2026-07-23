import { ref } from "vue";
import { useToast } from "@/lib/toast";
import { getErrorMessage } from "@/lib/error";

/**
 * Shared race-safe async op machinery for `useAsyncData`, `useListPage`,
 * `useFormSubmit`, and `useDelete`. Owns:
 *   - `loading` ref (mutated by the caller before/after each `run`)
 *   - `error` ref (cleared at the start of each `run`, populated on failure)
 *   - race-safe `seq` counter — only the latest call's result lands in refs
 *   - toast push for success / `getErrorMessage(err, fallbackError)` for failure
 *
 * The `run` function returns the result of `fn` on success, or `null` on
 * failure (after pushing the toast). Callers that need to inspect the
 * error or read the raw result use the refs exposed below.
 */
export function useAsyncOp(opts: { fallbackError: string }) {
  const loading = ref(false);
  const error = ref("");
  const { push } = useToast();
  let seq = 0;

  async function run<T>(fn: () => Promise<T>, successMsg?: string): Promise<T | null> {
    const id = ++seq;
    loading.value = true;
    error.value = "";
    try {
      const result = await fn();
      if (id === seq && successMsg) push(successMsg, "success");
      return result;
    } catch (err) {
      if (id === seq) {
        const msg = getErrorMessage(err, opts.fallbackError);
        error.value = msg;
        push(msg, "error");
      }
      return null;
    } finally {
      if (id === seq) loading.value = false;
    }
  }

  return { loading, error, run };
}
