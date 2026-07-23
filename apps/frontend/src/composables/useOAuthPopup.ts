import { onBeforeUnmount, ref } from "vue";
import { getErrorMessage } from "@/lib/error";

export function useOAuthPopup() {
  const loading = ref(false);
  const errorMessage = ref("");
  let timer: number | null = null;

  function open(url: string, onSuccess: () => void, windowName = "oauth-popup") {
    errorMessage.value = "";
    loading.value = true;
    try {
      const popup = window.open(url, windowName, "width=600,height=700");
      if (!popup) {
        errorMessage.value = "Popup blocked. Please allow popups for this site.";
        loading.value = false;
        return;
      }
      if (timer) clearInterval(timer);
      timer = setInterval(() => {
        if (popup.closed) {
          if (timer) {
            clearInterval(timer);
            timer = null;
          }
          loading.value = false;
          onSuccess();
        }
      }, 500);
    } catch (err) {
      errorMessage.value = getErrorMessage(err, "Failed to open popup");
      loading.value = false;
    }
  }

  function cleanup() {
    if (timer) {
      clearInterval(timer);
      timer = null;
    }
  }

  onBeforeUnmount(cleanup);

  return { open, loading, errorMessage, cleanup };
}
