<script setup lang="ts">
import { computed } from "vue";
import { formatTime, formatTimeOnly } from "@/lib/time";
import { useLongPress, type LongPressPosition } from "@/composables/useLongPress";
import Avatar from "./Avatar.vue";
import MarkdownRenderer from "./MarkdownRenderer.vue";

defineOptions({ name: "ChatMessageRow", inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    id: string;
    /**
     * Small colored chip class shown next to the message meta. Replaces the
     * old `borderClass` prop and is intentionally NOT applied as a left
     * accent border strip (AGENTS.md forbids colored `border-l` rails).
     * Pass a non-rail class such as `bg-purple-500/15 text-purple-700`.
     */
    indicatorClass?: string;
    highlightClass?: string;
    avatarSrc?: string;
    avatarLetter?: string;
    avatarBg: string;
    avatarTitle: string;
    displayName: string;
    createdAt: string;
    edited?: boolean;
    content: string;
    searchQuery?: string;
    internal?: boolean;
    variant?: "thread" | "dm";
    align?: "left" | "right";
    replyToText?: string;
    replyToAuthor?: string;
  }>(),
  {
    indicatorClass: "",
    highlightClass: "",
    edited: false,
    internal: false,
    variant: "thread",
    align: "left",
    replyToText: "",
    replyToAuthor: "",
  },
);

const emit = defineEmits<{
  "context-menu": [payload: { id: string; clientX: number; clientY: number }];
}>();

const isDm = computed(() => props.variant === "dm");
const isRight = computed(() => isDm.value && props.align === "right");

const containerClasses = computed(() => {
  const base = [
    "group",
    "relative",
    "transition-colors",
    "py-2.5",
    "px-4",
    "select-none",
    "touch-pan-y",
  ];

  if (isDm.value) {
    base.push("rounded", "max-w-[80%]");
    if (isRight.value) {
      base.push("rounded-br-sm", "ml-auto");
      if (props.internal) {
        base.push("bg-[var(--bg-internal-note)]", "border", "border-[var(--border-internal-note)]");
      } else {
        base.push("bg-[var(--accent)]/10", "border", "border-[var(--accent)]/20");
      }
    } else {
      base.push("rounded-bl-sm");
      if (props.internal) {
        base.push("bg-[var(--bg-internal-note)]", "border", "border-[var(--border-internal-note)]");
      } else {
        base.push("bg-[var(--bg-card)]", "border", "border-[var(--border-primary)]");
      }
    }
  } else {
    // Thread rows get a single neutral left edge — no colored rail.
    base.push("rounded", "border-l-2", "border-l-[var(--border-primary)]");
    base.push(props.internal ? "bg-[var(--bg-internal-note)]" : "bg-[var(--bg-card)]");
  }

  if (props.highlightClass) {
    base.push(props.highlightClass);
  }

  return base;
});

const indicatorDotClass = computed(() => {
  // Extract just the bg color so the dot doesn't pick up the text color.
  return props.indicatorClass.split(/\s+/).find((c) => c.startsWith("bg-")) ?? "";
});

const longPress = useLongPress({
  onTrigger(position: LongPressPosition) {
    emitContextMenu(position);
  },
});

function emitContextMenu(position: LongPressPosition) {
  emit("context-menu", {
    id: props.id,
    clientX: position.clientX,
    clientY: position.clientY,
  });
}

function onContextMenu(event: MouseEvent) {
  if (longPress.shouldSuppressMouseEvent(event)) {
    event.preventDefault();
    return;
  }
  event.preventDefault();
  emitContextMenu({ clientX: event.clientX, clientY: event.clientY });
}
</script>

<template>
  <div
    :data-chat-msg-id="id"
    :class="containerClasses"
    v-bind="$attrs"
    @pointerdown="longPress.onPointerDown"
    @pointermove="longPress.onPointerMove"
    @pointerup="longPress.onPointerUp"
    @pointercancel="longPress.onPointerCancel"
    @contextmenu="onContextMenu"
  >
    <div
      v-if="$slots.actions"
      class="absolute right-2 top-2 z-10 flex items-center gap-1 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100"
    >
      <slot name="actions" />
    </div>
    <slot />
    <div class="flex items-start gap-3">
      <Avatar
        v-if="!isRight"
        :src="avatarSrc"
        :letter="avatarLetter"
        :bg-class="avatarBg"
        :title="avatarTitle"
        class="mt-0.5"
      />
      <div class="min-w-0 flex-1">
        <div class="mb-0.5 flex flex-wrap items-center gap-1.5">
          <span
            v-if="indicatorDotClass"
            :class="['inline-block h-2 w-2 shrink-0 rounded-full', indicatorDotClass]"
            aria-hidden="true"
          />
          <span class="text-sm font-semibold text-[var(--text-primary)]">
            {{ displayName }}
          </span>
          <slot name="meta-extras" />
          <span class="text-xs text-[var(--text-muted)]" :title="formatTime(createdAt)">
            {{ formatTimeOnly(createdAt) }}
          </span>
          <span v-if="edited" class="text-xs italic text-[var(--text-muted)]"> (edited) </span>
        </div>
        <div
          v-if="replyToText"
          class="mb-1.5 rounded-md border border-[var(--border-primary)] bg-[var(--bg-muted)]/40 px-2 py-1 text-xs text-[var(--text-muted)]"
        >
          <span v-if="replyToAuthor" class="font-semibold text-[var(--text-secondary)]">{{
            replyToAuthor
          }}</span>
          <p class="line-clamp-3 whitespace-pre-wrap break-words">{{ replyToText }}</p>
        </div>
        <MarkdownRenderer :content="content" :highlight-text="searchQuery" />
      </div>
    </div>
  </div>
</template>
