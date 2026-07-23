import type { OwnerThread, OwnerThreadMessage } from "@/lib/api";

export type OwnerThreadWireResponse = Partial<Omit<OwnerThread, "messages">> & {
  messages?: OwnerThreadMessage[];
  items?: OwnerThreadMessage[];
  total?: number;
};

export function normalizeOwnerThreadResponse(response: OwnerThreadWireResponse): OwnerThread {
  const messages = response.messages ?? response.items ?? [];
  return {
    ...response,
    messages,
  };
}
