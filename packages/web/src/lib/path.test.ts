import { describe, expect, it } from "vite-plus/test";

import { shortPath } from "~/lib/path.ts";

describe("shortPath", () => {
  it("shortens paths under home only at a path boundary", () => {
    expect(shortPath("/home/me/work/x", "/home/me")).toBe("~/work/x");
    expect(shortPath("/home/me", "/home/me")).toBe("~");
    expect(shortPath("/home/meadow/x", "/home/me")).toBe("/home/meadow/x");
    expect(shortPath("/private/tmp/home/x", "/tmp/home")).toBe("/private/tmp/home/x");
    expect(shortPath("/x", "")).toBe("/x");
  });
});
