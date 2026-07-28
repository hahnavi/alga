import { describe, expect, it } from "vitest";
import { ref, nextTick } from "vue";
import { useEntityPageHeader } from "../src/composables/useEntityPageHeader.ts";
import { pageHeader, clearPageHeader } from "../src/lib/pageHeader.ts";

type Row = { id: string; name: string };

describe("useEntityPageHeader", () => {
  it("syncs header title and badges on source change", async () => {
    clearPageHeader();
    const source = ref<Row | null>({ id: "1", name: "Alpha" });
    useEntityPageHeader<Row>({
      source,
      buildTitle: (r) => r.name,
      buildBadges: (r) => [{ text: r.id, cssClass: "bg-slate-100" }],
    });
    await nextTick();
    expect(pageHeader.value?.title).toBe("Alpha");
    expect(pageHeader.value?.badges).toEqual([{ text: "1", cssClass: "bg-slate-100" }]);
  });

  it("updates header when source ref changes", async () => {
    clearPageHeader();
    const source = ref<Row | null>(null);
    useEntityPageHeader<Row>({
      source,
      buildTitle: (r) => r.name,
    });
    await nextTick();
    expect(pageHeader.value).toBeNull();
    source.value = { id: "x", name: "Beta" };
    await nextTick();
    expect(pageHeader.value?.title).toBe("Beta");
  });

  it("clears the page header on unmount", async () => {
    clearPageHeader();
    const source = ref<Row | null>({ id: "2", name: "Gamma" });
    useEntityPageHeader<Row>({ source, buildTitle: (r) => r.name });
    await nextTick();
    expect(pageHeader.value?.title).toBe("Gamma");
    clearPageHeader();
    expect(pageHeader.value).toBeNull();
  });
});
