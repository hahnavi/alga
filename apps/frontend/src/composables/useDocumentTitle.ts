import { onBeforeUnmount, watchEffect } from "vue";

const DOCUMENT_TITLE_SUFFIX = "Alga";

function format(label: string): string {
  return label ? `${label} · ${DOCUMENT_TITLE_SUFFIX}` : DOCUMENT_TITLE_SUFFIX;
}

export function useDocumentTitle(label: () => string) {
  watchEffect(() => {
    document.title = format(label());
  });
  onBeforeUnmount(() => {
    document.title = DOCUMENT_TITLE_SUFFIX;
  });
}
