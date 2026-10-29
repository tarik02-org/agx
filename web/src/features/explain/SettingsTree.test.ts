import { describe, expect, it } from "vite-plus/test";

import { settingsTree } from "~/features/explain/SettingsTree.tsx";
import { create } from "@bufbuild/protobuf";
import { HarnessSettingSchema } from "~/gen/agx/v1/agx_pb.ts";

const setting = (path: ReadonlyArray<string>, valueJson: string) =>
  create(HarnessSettingSchema, { path: [...path], valueJson });

describe("settingsTree", () => {
  it("nests settings under their key path", () => {
    const tree = settingsTree([
      setting(["model_provider"], '"cpa"'),
      setting(["model_providers", "cpa", "name"], '"CPA"'),
      setting(["model_providers", "cpa", "base_url"], '"http://127.0.0.1:8317/v1"'),
    ]);
    expect([...tree.children.keys()]).toEqual(["model_provider", "model_providers"]);
    const cpa = tree.children.get("model_providers")?.children.get("cpa");
    expect([...(cpa?.children.keys() ?? [])]).toEqual(["name", "base_url"]);
    expect(cpa?.children.get("name")?.setting?.valueJson).toBe('"CPA"');
  });
});
