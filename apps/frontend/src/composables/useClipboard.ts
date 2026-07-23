import { useToast } from "@/lib/toast";

export function useClipboard() {
  const { push } = useToast();

  async function copyToClipboard(text: string, successMsg = "Copied", errorMsg = "Failed to copy") {
    try {
      await navigator.clipboard.writeText(text);
      push(successMsg, "success");
    } catch {
      push(errorMsg, "error");
    }
  }

  return { copyToClipboard };
}
