import { computed, type Ref } from "vue";

type StringLike = string | null | undefined;

type StringField<T> = {
  [K in keyof T]: T[K] extends StringLike ? K : never;
}[keyof T];

type ListFilterField<T> = StringField<T> | ((item: T) => string | undefined);

interface ListFilterOptions<T> {
  sort?: (a: T, b: T) => number;
}

export function useListFilter<T>(
  source: Ref<T[]>,
  fields: ReadonlyArray<ListFilterField<T>>,
  query: Ref<string>,
  options: ListFilterOptions<T> = {},
): Ref<T[]> {
  return computed<T[]>(() => {
    const list: T[] = (source.value as T[] | null | undefined) ?? [];
    const q = query.value.trim().toLowerCase();
    if (!q) {
      return options.sort ? list.slice().sort(options.sort) : list;
    }
    const matches = (item: T): boolean => {
      for (const field of fields) {
        const value = typeof field === "function" ? field(item) : item[field];
        if (value == null) continue;
        if (String(value).toLowerCase().includes(q)) return true;
      }
      return false;
    };
    const filtered = list.filter(matches);
    return options.sort ? filtered.sort(options.sort) : filtered;
  });
}
