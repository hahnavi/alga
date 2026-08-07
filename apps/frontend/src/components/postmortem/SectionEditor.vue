<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { EditorContent, useEditor } from "@tiptap/vue-3";
import type { Editor } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import Placeholder from "@tiptap/extension-placeholder";
import Link from "@tiptap/extension-link";
import Underline from "@tiptap/extension-underline";
import { Markdown } from "tiptap-markdown";
import MarkdownToolbar from "@/components/ui/markdown/MarkdownToolbar.vue";
import "@/components/ui/MarkdownEditor.css";
import "./SectionEditor.css";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    disabled?: boolean;
    minHeight?: string;
  }>(),
  {
    placeholder: "Write in markdown — lists, **bold**, `code`, links…",
    disabled: false,
    minHeight: "140px",
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

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
      class: "markdown-editor-content section-editor__content",
    },
  },
  onUpdate: ({ editor: e }) => {
    emit("update:modelValue", getMarkdown(e));
  },
});

watch(
  () => props.modelValue,
  (newVal) => {
    if (!editor.value) return;
    if (newVal !== getMarkdown(editor.value)) {
      editor.value.commands.setContent(newVal);
    }
  },
);

watch(
  () => props.disabled,
  (disabled) => {
    editor.value?.setEditable(!disabled);
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

function focus() {
  editor.value?.commands.focus();
}

defineExpose({ focus });
</script>

<template>
  <div class="section-editor" :class="{ 'section-editor--disabled': disabled }">
    <div class="section-editor__toolbar">
      <MarkdownToolbar :editor="editor" :disabled="disabled" />
    </div>
    <div class="section-editor__body">
      <EditorContent v-if="editorReady && editor" :editor="editor" />
    </div>
  </div>
</template>
