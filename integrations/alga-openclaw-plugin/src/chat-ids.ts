/** Normalize inbound `chat_id` to canonical `investigation_<id>`. */
export function normalizeInvestigationChatId(chatId: string): string {
  const c = chatId.trim();
  if (c.startsWith("investigation_")) {
    return c;
  }
  return `investigation_${c}`;
}

/** Strip the `investigation_` prefix before sending JSON to Alga. */
export function stripInvestigationChatPrefix(chatId: string): string {
  if (chatId.startsWith("investigation_")) {
    return chatId.slice("investigation_".length);
  }
  return chatId;
}
