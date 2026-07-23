import type { Editor } from "@tiptap/core";

/**
 * Walk a TipTap editor's document and collect the IDs of every mention node.
 * Shared by chat/coordination editors that submit `mentions` with the message
 * body to the backend. Centralised so the schema is not re-derived per page.
 */
export function extractMentionIds(editor: Editor | null | undefined): string[] {
  if (!editor) return [];
  const ids: string[] = [];
  editor.state.doc.descendants((node) => {
    if (node.type.name === "mention") {
      const id = node.attrs.id;
      if (typeof id === "string" && id) ids.push(id);
    }
    return true;
  });
  return ids;
}
