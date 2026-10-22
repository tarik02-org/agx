import { ChevronRightIcon } from "lucide-react";

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "~/components/ui/collapsible.tsx";
import type { HarnessSetting } from "~/gen/agx/v1/agx_pb.ts";
import { shortPath } from "~/lib/path.ts";

/** Settings grouped by key path: `model_providers › cpa › base_url` nests under its parents. */
interface SettingNode {
  readonly key: string;
  readonly children: Map<string, SettingNode>;
  setting: HarnessSetting | undefined;
}

const node = (key: string): SettingNode => ({ key, children: new Map(), setting: undefined });

export const settingsTree = (settings: ReadonlyArray<HarnessSetting>): SettingNode => {
  const root = node("");
  for (const setting of settings) {
    let current = root;
    for (const key of setting.path) {
      let child = current.children.get(key);
      if (child === undefined) {
        child = node(key);
        current.children.set(key, child);
      }
      current = child;
    }
    current.setting = setting;
  }
  return root;
};

// A group of one child reads better as one line: `model_providers.cpa`.
const collapse = (start: SettingNode): { readonly label: string; readonly node: SettingNode } => {
  let label = start.key;
  let current = start;
  while (current.setting === undefined && current.children.size === 1) {
    const [only] = current.children.values();
    if (only === undefined || only.setting !== undefined) {
      break;
    }
    label = `${label}.${only.key}`;
    current = only;
  }
  return { label, node: current };
};

function Leaf({ setting, home }: { readonly setting: HarnessSetting; readonly home: string }) {
  const key = setting.path.at(-1) ?? "";
  return (
    <li className="flex items-baseline gap-2 py-0.5 font-mono text-xs">
      <span className="text-muted-foreground">{key}</span>
      <span className="break-all">{setting.valueJson}</span>
      {setting.source !== undefined && (
        <span className="ml-auto shrink-0 text-muted-foreground">
          {shortPath(setting.source.path, home)}:{setting.source.line}
        </span>
      )}
    </li>
  );
}

function Group({ start, home }: { readonly start: SettingNode; readonly home: string }) {
  if (start.setting !== undefined) {
    return <Leaf setting={start.setting} home={home} />;
  }
  const { label, node } = collapse(start);
  return (
    <li>
      <Collapsible defaultOpen className="group/setting">
        <CollapsibleTrigger className="flex items-center gap-1 py-0.5 font-mono text-xs">
          <ChevronRightIcon className="size-3 transition-transform group-data-[open]/setting:rotate-90" />
          {label}
        </CollapsibleTrigger>
        <CollapsibleContent>
          <ul className="ml-1.5 border-l pl-3">
            {[...node.children.values()].map((child) => (
              <Group key={child.key} start={child} home={home} />
            ))}
          </ul>
        </CollapsibleContent>
      </Collapsible>
    </li>
  );
}

/** A harness's resolved settings as nested groups, each value with where it was set. */
export function SettingsTree({
  settings,
  home,
}: {
  readonly settings: ReadonlyArray<HarnessSetting>;
  readonly home: string;
}) {
  return (
    <ul className="flex flex-col">
      {[...settingsTree(settings).children.values()].map((child) => (
        <Group key={child.key} start={child} home={home} />
      ))}
    </ul>
  );
}
