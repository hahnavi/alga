import { computed, onBeforeUnmount, ref } from "vue";

export function useTypingIndicator(options?: { timeoutMs?: number }) {
  const typingSource = ref<string | null>(null);
  const typingAgentType = ref<string | null>(null);
  let timeout: number | null = null;
  const timeoutMs = options?.timeoutMs ?? 5000;

  const isTyping = computed(() => typingSource.value !== null);

  function setTyping(source = "agent", agentType?: string) {
    typingSource.value = source;
    typingAgentType.value = agentType?.trim() ? agentType.trim() : null;
    if (timeout) clearTimeout(timeout);
    timeout = setTimeout(() => {
      typingSource.value = null;
      typingAgentType.value = null;
      timeout = null;
    }, timeoutMs);
  }

  function clearTyping() {
    if (timeout) {
      clearTimeout(timeout);
      timeout = null;
    }
    typingSource.value = null;
    typingAgentType.value = null;
  }

  onBeforeUnmount(() => {
    clearTyping();
  });

  return { typingSource, typingAgentType, isTyping, setTyping, clearTyping };
}
