<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { EditorContent, useEditor } from "@tiptap/vue-3";
import type { Editor } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import Placeholder from "@tiptap/extension-placeholder";
import Mention from "@tiptap/extension-mention";
import Link from "@tiptap/extension-link";
import Underline from "@tiptap/extension-underline";
import { Markdown } from "tiptap-markdown";
import type { AgentTokenRow, UserInfo } from "@/lib/api";
import MarkdownMentionPopup, {
  type MentionItem,
} from "@/components/ui/markdown/MarkdownMentionPopup.vue";
import MarkdownToolbar from "@/components/ui/markdown/MarkdownToolbar.vue";
import MarkdownSendMenu from "@/components/ui/markdown/MarkdownSendMenu.vue";
import "./MarkdownEditor.css";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    disabled?: boolean;
    users?: UserInfo[];
    agents?: AgentTokenRow[];
    enableInternalNote?: boolean;
    showSendButton?: boolean;
  }>(),
  {
    placeholder: "Add a comment... (@ to mention, Ctrl+Enter to send)",
    disabled: false,
    users: () => [],
    agents: () => [],
    enableInternalNote: true,
    showSendButton: true,
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
  submit: [];
  submitInternal: [];
}>();

const query = ref("");
const selectedIndex = ref(0);
const showMentionPopup = ref(false);
const mentionRange = ref<{ from: number; to: number } | null>(null);

const isEditorEmpty = computed(() => !props.modelValue.trim());

const filteredUsers = computed<MentionItem[]>(() => {
  const items: MentionItem[] = [];
  for (const u of props.users ?? []) {
    items.push({
      id: `user:${u.id}`,
      label: u.full_name || u.email,
      mentionType: "user",
      subtitle: u.email,
    });
  }
  for (const a of props.agents ?? []) {
    items.push({
      id: `agent:${a.id}`,
      label: a.name,
      mentionType: "agent",
      subtitle: a.agent_type ? `Agent (${a.agent_type})` : "Agent",
    });
  }
  if (!query.value) return items;
  const q = query.value.toLowerCase();
  return items.filter(
    (i) =>
      i.label.toLowerCase().includes(q) || (i.subtitle && i.subtitle.toLowerCase().includes(q)),
  );
});

function getMarkdown(editor: Editor): string {
  return (
    (editor.storage as { markdown?: { getMarkdown: () => string } }).markdown?.getMarkdown() ?? ""
  );
}

const editorReady = ref(false);
const editor = useEditor({
  content: props.modelValue,
  extensions: [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
      link: false,
      underline: false,
    }),
    Underline,
    Link.configure({
      openOnClick: false,
      HTMLAttributes: { class: "editor-link" },
    }),
    Placeholder.configure({
      placeholder: props.placeholder,
    }),
    Mention.extend({
      addStorage() {
        return {
          markdown: {
            serialize(
              state: { write: (s: string) => void },
              node: { attrs: Record<string, string> },
            ) {
              const label = node.attrs.label || node.attrs.id || "mention";
              const id = node.attrs.id || "";
              state.write(`[@${label}](${id})`);
            },
          },
        };
      },
    }).configure({
      suggestion: {
        char: "@",
        items: ({ query: q }: { query: string }) => {
          const items: MentionItem[] = [];
          for (const u of props.users ?? []) {
            items.push({ id: `user:${u.id}`, label: u.full_name || u.email, mentionType: "user" });
          }
          for (const a of props.agents ?? []) {
            items.push({ id: `agent:${a.id}`, label: a.name, mentionType: "agent" });
          }
          if (!q) return items.slice(0, 8);
          const lower = q.toLowerCase();
          return items.filter((i) => i.label.toLowerCase().includes(lower)).slice(0, 8);
        },
        render: () => {
          return {
            onStart: (props: { query: string; range: { from: number; to: number } }) => {
              query.value = "";
              selectedIndex.value = 0;
              showMentionPopup.value = true;
              mentionRange.value = { from: props.range.from, to: props.range.to };
            },
            onUpdate: (props: { query: string; range: { from: number; to: number } }) => {
              query.value = props.query;
              selectedIndex.value = 0;
              mentionRange.value = { from: props.range.from, to: props.range.to };
            },
            onExit: () => {
              showMentionPopup.value = false;
              query.value = "";
            },
            onKeyDown: (props: { event: KeyboardEvent }) => {
              if (!showMentionPopup.value) return false;

              if (props.event.key === "ArrowDown") {
                selectedIndex.value = (selectedIndex.value + 1) % filteredUsers.value.length;
                return true;
              }
              if (props.event.key === "ArrowUp") {
                selectedIndex.value =
                  (selectedIndex.value - 1 + filteredUsers.value.length) %
                  filteredUsers.value.length;
                return true;
              }
              if (props.event.key === "Enter" || props.event.key === "Tab") {
                if (filteredUsers.value.length > 0) {
                  const user = filteredUsers.value[selectedIndex.value];
                  selectMentionUser(user);
                }
                return true;
              }
              if (props.event.key === "Escape") {
                showMentionPopup.value = false;
                return true;
              }
              return false;
            },
          };
        },
      },
      HTMLAttributes: { class: "mention-node" },
      renderHTML({ node }: { node: { attrs: Record<string, string> } }) {
        return ["span", { class: "mention-node" }, `@${node.attrs.label || node.attrs.id}`];
      },
    }),
    Markdown.configure({
      html: false,
      tightLists: true,
      breaks: true,
      linkify: false,
      transformPastedText: true,
      transformCopiedText: true,
    }),
  ],
  editorProps: {
    attributes: {
      class: "markdown-editor-content",
    },
    handleKeyDown: (_view: unknown, event: KeyboardEvent) => {
      if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
        event.preventDefault();
        emit("submit");
        return true;
      }
      return false;
    },
  },
  onUpdate: ({ editor: e }) => {
    const md = getMarkdown(e);
    emit("update:modelValue", md);
  },
});

function selectMentionUser(item: MentionItem) {
  if (!editor.value) return;
  const cssClass =
    item.mentionType === "agent" ? "mention-node mention-node--agent" : "mention-node";
  const chain = editor.value.chain().focus();
  if (mentionRange.value) {
    chain.deleteRange(mentionRange.value);
  }
  chain
    .insertContent({
      type: "mention",
      attrs: { id: item.id, label: item.label, class: cssClass },
    })
    .insertContent(" ")
    .run();
  showMentionPopup.value = false;
  query.value = "";
  mentionRange.value = null;
}

watch(
  () => props.modelValue,
  (newVal) => {
    if (!editor.value) return;
    const currentMd = getMarkdown(editor.value);
    if (newVal !== currentMd) {
      const pos = editor.value.state.selection.anchor;
      editor.value.commands.setContent(newVal);
      try {
        const maxPos = editor.value.state.doc.content.size;
        if (pos <= maxPos) {
          editor.value.commands.setTextSelection(Math.min(pos, maxPos));
        }
      } catch {
        // ignore position errors
      }
    }
  },
);

watch(
  () => props.disabled,
  (disabled) => {
    if (editor.value) {
      editor.value.setEditable(!disabled);
    }
  },
);

onMounted(() => {
  if (editor.value && props.disabled) {
    editor.value.setEditable(false);
  }
  editorReady.value = true;
});

onBeforeUnmount(() => {
  editorReady.value = false;
  editor.value?.destroy();
});

function insertQuote(quote: string) {
  if (!editor.value) return;
  const lines = quote
    .split("\n")
    .map((line) => `> ${line}`)
    .join("\n");
  editor.value
    .chain()
    .focus()
    .insertContentAt(0, lines + "\n\n")
    .run();
}

function focus() {
  editor.value?.commands.focus();
}

function getMentionIds(): string[] {
  if (!editor.value) return [];
  const ids = new Set<string>();
  editor.value.state.doc.descendants((node: { type: { name: string }; attrs: { id?: string } }) => {
    if (node.type.name === "mention" && node.attrs.id) ids.add(node.attrs.id);
  });
  return [...ids];
}

defineExpose({ insertQuote, focus, getMentionIds });
</script>

<template>
  <div class="markdown-editor" :class="{ 'markdown-editor--disabled': disabled }">
    <div class="markdown-editor__body">
      <MarkdownMentionPopup
        v-if="showMentionPopup && filteredUsers.length > 0"
        :items="filteredUsers"
        :selected-index="selectedIndex"
        @select="selectMentionUser"
        @hover="(i) => (selectedIndex = i)"
      />
      <EditorContent v-if="editorReady && editor" :editor="editor" />
    </div>

    <div class="markdown-editor__toolbar">
      <MarkdownToolbar :editor="editor" :disabled="disabled" />
      <div class="markdown-editor__toolbar-spacer" />
      <MarkdownSendMenu
        v-if="showSendButton"
        :disabled="disabled"
        :enable-internal-note="enableInternalNote"
        :is-editor-empty="isEditorEmpty"
        @send="emit('submit')"
        @send-internal="emit('submitInternal')"
      />
    </div>
  </div>
</template>
