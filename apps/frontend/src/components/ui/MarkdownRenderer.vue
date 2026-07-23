<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import MarkdownIt from "markdown-it";
import DOMPurify from "dompurify";

const props = defineProps<{
  content: string;
  highlightText?: string;
}>();

const router = useRouter();

const md = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: true,
  breaks: true,
});

const defaultTextRule = md.renderer.rules.text!;

md.renderer.rules.text = function (tokens, idx, options, env, self) {
  let result = defaultTextRule(tokens, idx, options, env, self);
  if (env?.highlightText) {
    const escaped = (env.highlightText as string).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const re = new RegExp(`(${escaped})`, "gi");
    result = result.replace(re, '<mark class="search-highlight">$1</mark>');
  }
  result = result.replace(/(^|\s)@([\w.-]+)/g, '$1<span class="mention-highlight">@$2</span>');
  return result;
};

const defaultLinkOpenRule =
  md.renderer.rules.link_open ||
  ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options));

md.renderer.rules.link_open = function (tokens, idx, options, env, self) {
  const href = tokens[idx].attrGet("href") as string | null;
  if (href && (href.startsWith("user:") || href.startsWith("group:"))) {
    const isGroup = href.startsWith("group:");
    tokens[idx].attrSet(
      "class",
      isGroup ? "mention-highlight mention-highlight--group" : "mention-highlight",
    );
    tokens[idx].attrSet("href", "#");
  }
  return defaultLinkOpenRule(tokens, idx, options, env, self);
};

let hookInstalled = false;
function ensureSanitizeHook() {
  if (hookInstalled) return;
  hookInstalled = true;
  DOMPurify.addHook("afterSanitizeAttributes", (node: Element) => {
    if (node.tagName === "A") {
      const href = node.getAttribute("href");
      if (!href) return;
      // Block javascript: and other dangerous schemes that DOMPurify's default
      // URI allowlist may still permit through depending on the input format.
      if (/^\s*(javascript|data|vbscript):/i.test(href)) {
        node.removeAttribute("href");
        return;
      }
      if (!href.startsWith("#") && !href.startsWith("/") && !href.startsWith("mailto:")) {
        node.setAttribute("target", "_blank");
        node.setAttribute("rel", "noopener noreferrer");
      }
    }
  });
}

ensureSanitizeHook();

const rendered = computed(() =>
  DOMPurify.sanitize(md.render(props.content || "", { highlightText: props.highlightText }), {
    FORBID_TAGS: ["form", "input", "button", "select", "textarea", "style"],
    ADD_ATTR: ["target"],
    ADD_TAGS: ["mark"],
    ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto|tel|user|group|#|\/)|\s*#)/i,
  }),
);

function handleClick(e: MouseEvent) {
  const target = e.target as HTMLElement;
  const anchor = target.closest("a");
  if (!anchor) return;
  const href = anchor.getAttribute("href");
  if (!href || !href.startsWith("/")) return;
  e.preventDefault();
  router.push(href);
}
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -->
  <div class="markdown-body" v-html="rendered" @click="handleClick" />
</template>
