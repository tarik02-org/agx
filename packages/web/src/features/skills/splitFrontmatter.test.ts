import { describe, expect, it } from "vite-plus/test";

import { splitFrontmatter } from "~/features/skills/SkillDocument.tsx";

describe("splitFrontmatter", () => {
  it("splits front matter from the body", () => {
    expect(splitFrontmatter("---\nname: a\ndescription: b\n---\n# A\n")).toEqual({
      frontmatter: "name: a\ndescription: b",
      body: "# A\n",
    });
  });

  it("keeps a body without front matter", () => {
    expect(splitFrontmatter("# A\n---\n")).toEqual({ frontmatter: "", body: "# A\n---\n" });
  });

  it("splits an empty front matter block", () => {
    expect(splitFrontmatter("---\n---\nBody.\n")).toEqual({ frontmatter: "", body: "Body.\n" });
  });
});
