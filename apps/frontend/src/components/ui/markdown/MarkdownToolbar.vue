<script setup lang="ts">
import { ref } from "vue";
import type { Editor } from "@tiptap/core";
import {
  Bold,
  Code,
  Code2,
  Heading1,
  Heading2,
  Heading3,
  Italic,
  Link,
  List,
  ListOrdered,
  MoreHorizontal,
  Strikethrough,
  TextQuote,
  Underline,
} from "@lucide/vue";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import Input from "@/components/ui/Input.vue";
import Modal from "@/components/ui/Modal.vue";
import Button from "@/components/ui/Button.vue";

const props = defineProps<{
  editor: Editor | null | undefined;
  disabled: boolean;
}>();

const showHeadingPopup = ref(false);
const showMorePopup = ref(false);
const showLinkModal = ref(false);
const linkUrl = ref("");

const headingWrapperRef = ref<HTMLElement | null>(null);
const moreWrapperRef = ref<HTMLElement | null>(null);

useDropdownLifecycle(showHeadingPopup, headingWrapperRef);
useDropdownLifecycle(showMorePopup, moreWrapperRef);

function toggleBold() {
  props.editor?.chain().focus().toggleBold().run();
}
function toggleItalic() {
  props.editor?.chain().focus().toggleItalic().run();
}
function toggleStrike() {
  props.editor?.chain().focus().toggleStrike().run();
}
function toggleUnderline() {
  props.editor?.chain().focus().toggleUnderline().run();
}
function toggleCode() {
  props.editor?.chain().focus().toggleCode().run();
}
function toggleCodeBlock() {
  props.editor?.chain().focus().toggleCodeBlock().run();
}
function toggleBlockquote() {
  props.editor?.chain().focus().toggleBlockquote().run();
}
function toggleBulletList() {
  props.editor?.chain().focus().toggleBulletList().run();
}
function toggleOrderedList() {
  props.editor?.chain().focus().toggleOrderedList().run();
}
function setHeading(level: 1 | 2 | 3) {
  props.editor?.chain().focus().toggleHeading({ level }).run();
  showHeadingPopup.value = false;
}
function setLink() {
  if (!props.editor) return;
  const previous = props.editor.getAttributes("link").href;
  linkUrl.value = previous || "https://";
  showLinkModal.value = true;
  showMorePopup.value = false;
}

function cancelLink() {
  showLinkModal.value = false;
}

function applyLink() {
  if (!props.editor) return;
  const trimmed = linkUrl.value.trim();
  if (trimmed === "") {
    props.editor.chain().focus().extendMarkRange("link").unsetLink().run();
    showLinkModal.value = false;
    return;
  }
  if (!/^(https?:\/\/|mailto:|\/|#)/i.test(trimmed)) return;
  props.editor.chain().focus().extendMarkRange("link").setLink({ href: trimmed }).run();
  showLinkModal.value = false;
}

function isHeadingActive(level: number): boolean {
  return props.editor?.isActive("heading", { level }) ?? false;
}

function isAnyHeadingActive(): boolean {
  return isHeadingActive(1) || isHeadingActive(2) || isHeadingActive(3);
}

function toggleHeadingPopup() {
  showHeadingPopup.value = !showHeadingPopup.value;
}
</script>

<template>
  <div class="markdown-editor__toolbar-group">
    <button
      type="button"
      class="markdown-editor__btn"
      :class="{ 'markdown-editor__btn--active': editor?.isActive('bold') }"
      :disabled="disabled"
      title="Bold (Ctrl+B)"
      @click="toggleBold"
    >
      <Bold class="h-4 w-4" aria-hidden="true" />
    </button>
    <button
      type="button"
      class="markdown-editor__btn"
      :class="{ 'markdown-editor__btn--active': editor?.isActive('italic') }"
      :disabled="disabled"
      title="Italic (Ctrl+I)"
      @click="toggleItalic"
    >
      <Italic class="h-4 w-4" aria-hidden="true" />
    </button>
    <button
      type="button"
      class="markdown-editor__btn"
      :class="{ 'markdown-editor__btn--active': editor?.isActive('underline') }"
      :disabled="disabled"
      title="Underline (Ctrl+U)"
      @click="toggleUnderline"
    >
      <Underline class="h-4 w-4" aria-hidden="true" />
    </button>
    <button
      type="button"
      class="markdown-editor__btn"
      :class="{ 'markdown-editor__btn--active': editor?.isActive('strike') }"
      :disabled="disabled"
      title="Strikethrough"
      @click="toggleStrike"
    >
      <Strikethrough class="h-4 w-4" aria-hidden="true" />
    </button>
  </div>

  <div class="markdown-editor__toolbar-separator" />

  <div class="markdown-editor__toolbar-group">
    <div ref="headingWrapperRef" class="markdown-editor__heading-wrapper">
      <button
        type="button"
        class="markdown-editor__btn"
        :class="{ 'markdown-editor__btn--active': isAnyHeadingActive() }"
        :disabled="disabled"
        title="Heading"
        @click="toggleHeadingPopup"
      >
        H
      </button>
      <div
        v-if="showHeadingPopup"
        class="markdown-editor__heading-popup"
        role="group"
        aria-label="Heading level"
      >
        <button
          type="button"
          class="markdown-editor__heading-option"
          :class="{ 'markdown-editor__heading-option--active': isHeadingActive(1) }"
          title="Heading 1"
          @click="setHeading(1)"
        >
          <Heading1 class="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="markdown-editor__heading-option"
          :class="{ 'markdown-editor__heading-option--active': isHeadingActive(2) }"
          title="Heading 2"
          @click="setHeading(2)"
        >
          <Heading2 class="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="markdown-editor__heading-option"
          :class="{ 'markdown-editor__heading-option--active': isHeadingActive(3) }"
          title="Heading 3"
          @click="setHeading(3)"
        >
          <Heading3 class="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
    </div>
  </div>

  <div class="markdown-editor__toolbar-separator" />

  <div class="markdown-editor__toolbar-group">
    <div ref="moreWrapperRef" class="markdown-editor__more-wrapper">
      <button
        type="button"
        class="markdown-editor__btn"
        :class="{ 'markdown-editor__btn--active': showMorePopup }"
        :disabled="disabled"
        title="More formatting"
        @click="showMorePopup = !showMorePopup"
      >
        <MoreHorizontal class="h-4 w-4" aria-hidden="true" />
      </button>
      <div
        v-if="showMorePopup"
        class="markdown-editor__more-popup"
        role="group"
        aria-label="More formatting"
      >
        <button
          type="button"
          class="markdown-editor__more-option"
          :class="{ 'markdown-editor__more-option--active': editor?.isActive('code') }"
          title="Inline code"
          @click="
            toggleCode();
            showMorePopup = false;
          "
        >
          <Code class="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="markdown-editor__more-option"
          :class="{ 'markdown-editor__more-option--active': editor?.isActive('codeBlock') }"
          title="Code block"
          @click="
            toggleCodeBlock();
            showMorePopup = false;
          "
        >
          <Code2 class="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="markdown-editor__more-option"
          :class="{ 'markdown-editor__more-option--active': editor?.isActive('link') }"
          title="Link"
          @click="
            setLink();
            showMorePopup = false;
          "
        >
          <Link class="h-4 w-4" aria-hidden="true" />
        </button>
        <div class="markdown-editor__more-separator" />
        <button
          type="button"
          class="markdown-editor__more-option"
          :class="{ 'markdown-editor__more-option--active': editor?.isActive('bulletList') }"
          title="Bullet list"
          @click="
            toggleBulletList();
            showMorePopup = false;
          "
        >
          <List class="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="markdown-editor__more-option"
          :class="{ 'markdown-editor__more-option--active': editor?.isActive('orderedList') }"
          title="Numbered list"
          @click="
            toggleOrderedList();
            showMorePopup = false;
          "
        >
          <ListOrdered class="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="markdown-editor__more-option"
          :class="{ 'markdown-editor__more-option--active': editor?.isActive('blockquote') }"
          title="Blockquote"
          @click="
            toggleBlockquote();
            showMorePopup = false;
          "
        >
          <TextQuote class="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
    </div>
  </div>

  <Modal
    :open="showLinkModal"
    title="Add link"
    max-width="sm"
    :show-footer="false"
    @close="cancelLink"
  >
    <div class="space-y-3">
      <label for="markdown-link-url" class="block text-sm font-medium text-[var(--text-primary)]">
        URL
      </label>
      <Input
        id="markdown-link-url"
        v-model="linkUrl"
        placeholder="https://example.com"
        @keydown.enter.prevent="applyLink"
      />
      <p class="text-xs text-[var(--text-muted)]">
        Use https://, mailto:, /, or #. Leave empty to remove the link.
      </p>
      <div class="flex justify-end gap-2">
        <Button variant="outline" @click="cancelLink">Cancel</Button>
        <Button @click="applyLink">Apply</Button>
      </div>
    </div>
  </Modal>
</template>
