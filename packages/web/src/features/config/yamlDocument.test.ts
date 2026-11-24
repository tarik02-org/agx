import { describe, expect, it } from "vite-plus/test";

import {
  putFields,
  readList,
  readMap,
  readString,
  readValue,
  removeFields,
  takeFields,
  writeField,
  writeMap,
  writeValue,
} from "~/features/config/yamlDocument.ts";

const server = `# The docs server.
url: https://example.com/mcp # keep this
headers:
  X-Token: !secret docs-token
`;

describe("yamlDocument", () => {
  it("edits a field and keeps comments", () => {
    const next = writeField(server, ["description"], "Docs search");
    expect(next).toContain("# The docs server.");
    expect(next).toContain("# keep this");
    expect(readString(next, ["description"])).toBe("Docs search");
  });

  it("removes a field set empty", () => {
    expect(writeField(server, ["url"], "")).not.toContain("url:");
  });

  it("reads and writes tagged values", () => {
    expect(readValue(server, ["headers", "X-Token"])).toEqual({
      tag: "!secret",
      value: "docs-token",
    });
    const next = writeValue(server, ["headers", "X-Token"], { tag: "!env", value: "TOKEN" });
    expect(next).toContain("X-Token: !env TOKEN");
  });

  it("replaces a map's entries in order", () => {
    const next = writeMap(
      server,
      ["headers"],
      [
        { key: "A", value: { tag: "", value: "1" } },
        { key: "B", value: { tag: "!file", value: "~/token" } },
      ],
    );
    expect(readMap(next, ["headers"]).map((entry) => entry.key)).toEqual(["A", "B"]);
    expect(next).toContain("B: !file ~/token");
  });

  it("starts an empty definition", () => {
    expect(writeField("", ["command"], ["npx", "server"])).toBe("command:\n  - npx\n  - server\n");
  });

  it("puts back fields taken out, with their comments and tags", () => {
    const text =
      "url: https://example.com/mcp\nheaders:\n  # The token.\n  X-Token: !secret docs-token\n";
    const taken = takeFields(text, ["url", "headers"]);
    const back = putFields(removeFields(text, ["url", "headers"]), taken);
    expect(back).toContain("# The token.");
    expect(readValue(back, ["headers", "X-Token"])).toEqual({
      tag: "!secret",
      value: "docs-token",
    });
    expect(readString(back, ["url"])).toBe("https://example.com/mcp");
  });

  it("reads no row for an empty list item", () => {
    expect(readList("harnesses: [claude, ]\nargs:\n  - a\n  -\n", ["harnesses"])).toEqual([
      "claude",
    ]);
    expect(readList("args:\n  - a\n  -\n", ["args"])).toEqual(["a"]);
  });
});
