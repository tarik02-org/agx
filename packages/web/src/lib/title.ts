/** A page's document title, most specific part first; React hoists `<title>` into the head. */
export const pageTitle = (...parts: ReadonlyArray<string | undefined>): string =>
  [...parts.filter((part): part is string => part !== undefined && part !== ""), "agx"].join(" · ");
