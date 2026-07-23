import { useAsyncOp } from "@/composables/useAsyncOp";

export function useFormSubmit() {
  const op = useAsyncOp({ fallbackError: "Operation failed" });

  async function withSubmit(fn: () => Promise<unknown>, successMsg?: string) {
    return op.run(fn, successMsg);
  }

  return { submitting: op.loading, formError: op.error, withSubmit };
}
