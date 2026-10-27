import { useQueries } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { SearchIcon } from "lucide-react";
import { type ReactNode, useState } from "react";

import { NavTree, type TreeIcon } from "~/components/NavTree.tsx";
import { InputGroup, InputGroupAddon, InputGroupInput } from "~/components/ui/input-group.tsx";
import { Switch } from "~/components/ui/switch.tsx";
import { ToggleGroup, ToggleGroupItem } from "~/components/ui/toggle-group.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { DaemonError } from "~/daemon/error.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { useRunEffect } from "~/effect/react.tsx";
import { kinds } from "~/features/config/kinds.ts";
import { NewItemMenu } from "~/features/config/NewItemMenu.tsx";
import {
  definitionIcon,
  filesIcon,
  harnessIcon,
  namespaceIcon,
  sectionIcon,
  toolIcon,
} from "~/features/config/treeIcons.ts";
import { readList } from "~/features/config/yamlDocument.ts";
import { useInspectorDirectory } from "~/features/mcp/tools.ts";
import { mcpStates } from "~/features/runtime/states.ts";
import { useEnabledHere, useScopeDirectory } from "~/features/scope/scope.ts";
import type { Definition, GetSkillResponse, McpTools } from "~/gen/agx/v1/agx_pb.ts";
import { McpRuntime_State } from "~/gen/agx/v1/agx_pb.ts";
import { shortPath } from "~/lib/path.ts";

/** Kinds a scope selects; the others apply wherever they are defined. */
const scoped = new Set(["skills", "instructions", "mcp"]);

const label = (kind: string) => kinds.find((k) => k.kind === kind)?.label ?? kind;

/** One of a kind, in a sentence: lowercase, unless it is an acronym such as MCP. */
const one = (kind: string) => {
  const name = kinds.find((k) => k.kind === kind)?.one ?? kind;
  return /^.[a-z]/.test(name) ? name.toLowerCase() : name;
};

const join = (...parts: ReadonlyArray<string>) => parts.filter((part) => part !== "").join("/");

/** What the tree needs of a folder's query. */
interface Loading<A> {
  readonly data: A | undefined;
  readonly error: unknown;
}

type Target =
  | { readonly page: "skill"; readonly skill: string; readonly file?: string }
  | { readonly page: "server"; readonly server: string; readonly tool?: string }
  | { readonly page: "definition"; readonly kind: string; readonly id: string }
  | { readonly page: "file"; readonly file: string }
  | { readonly page: "new"; readonly kind: string };

/**
 * What the top of the tree is: definitions themselves, the configuration's bundles with what each
 * selects, or its files with what each defines. Within that, definitions can sit under their kind.
 */
type Grouping = "none" | "bundle" | "file";

const groupings: ReadonlyArray<{ readonly value: Grouping; readonly label: string }> = [
  { value: "none", label: "None" },
  { value: "bundle", label: "Bundle" },
  { value: "file", label: "File" },
];

const groupingKey = "agx.tree.grouping";
const byTypeKey = "agx.tree.byType";
const savedGrouping = (): Grouping => {
  const saved = globalThis.localStorage?.getItem(groupingKey);
  return saved === "bundle" || saved === "file" ? saved : "none";
};
const savedByType = () => globalThis.localStorage?.getItem(byTypeKey) !== "false";

const outsideBundles = "Not in a bundle";
const filesFolder = "Files";

/** The definitions a bundle selects, as kind/id keys; inline definitions name no ID. */
const bundleMembers = (bundle: Definition): ReadonlyArray<string> =>
  ["skills", "instructions", "mcp", "bundles"].flatMap((kind) =>
    readList(bundle.yaml, [kind]).map((id) => `${kind}/${id}`),
  );

/**
 * The configuration as one tree, grouped by bundle or file and split by type, or not, with IDs nesting at their
 * slashes (a package's skills sit under the package's name). Skills hold their files and MCP
 * servers their tools, loaded when first expanded; a server starts only then.
 */
export function ConfigTree() {
  const definitions = useDaemonQuery(
    queryKeys.definitions,
    Daemon.use((daemon) => daemon.definitions),
  );
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const directory = useScopeDirectory();
  const toolsDirectory = useInspectorDirectory() ?? "";
  const enabledHere = useEnabledHere();
  const [onlyHere, setOnlyHere] = useState(false);
  const [grouping, setGrouping] = useState<Grouping>(savedGrouping);
  const [byType, setByType] = useState(savedByType);
  const [query, setQuery] = useState("");
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set());
  const navigate = useNavigate();
  const search = useSearch({ strict: false });
  const run = useRunEffect();
  const home = configuration.data?.home ?? "";
  const filtering = directory !== "" && onlyHere && enabledHere !== undefined;

  const all = definitions.data ?? [];
  const shown = all.filter(
    (definition) =>
      !filtering ||
      !scoped.has(definition.kind) ||
      enabledHere.has(`${definition.kind}/${definition.id}`),
  );

  // Files sit under the root's directory, most of them; any other keeps its full path.
  const rootDirectory = (configuration.data?.root ?? "").replace(/[^/]*$/, "");
  const relative = (path: string) =>
    rootDirectory !== "" && path.startsWith(rootDirectory)
      ? path.slice(rootDirectory.length)
      : shortPath(path, home).replace(/^\//, "");

  // The groups a definition shows in: none, its file, or each bundle selecting it.
  const groupsOf = (definition: Definition): ReadonlyArray<string> => {
    switch (grouping) {
      case "none":
        return [""];
      case "file":
        return [
          definition.package === ""
            ? relative(definition.source?.path ?? "")
            : `package ${definition.package}`,
        ];
      case "bundle": {
        const key = `${definition.kind}/${definition.id}`;
        const holders = all.filter((b) => b.kind === "bundles" && bundleMembers(b).includes(key));
        // A bundle no other selects is a group of its own.
        if (definition.kind === "bundles" && holders.length === 0) {
          return [];
        }
        return holders.length === 0 ? [outsideBundles] : holders.map((b) => b.id);
      }
    }
  };
  // Without kinds' folders, an ID several kinds share says which one it is.
  const shared = new Set(
    all
      .filter((d) => all.some((other) => other.id === d.id && other.kind !== d.kind))
      .map((d) => d.id),
  );
  const nameOf = (definition: Definition) =>
    !byType && shared.has(definition.id)
      ? `${definition.id} (${one(definition.kind)})`
      : definition.id;
  const placesOf = (definition: Definition): ReadonlyArray<string> =>
    groupsOf(definition).map((group) =>
      join(group, byType ? label(definition.kind) : "", nameOf(definition)),
    );

  // Every skill and server has its query, run once one of its folders is expanded; one a page
  // already read, under the same key, shows at once, with the icon its server declares.
  const allSkills = shown.filter((d) => d.kind === "skills");
  const allServers = shown.filter((d) => d.kind === "mcp");
  const isExpanded = (definition: Definition) =>
    placesOf(definition).some((path) => expanded.has(path));
  const skills = useQueries({
    queries: allSkills.map((d) => ({
      queryKey: queryKeys.skill(d.id),
      queryFn: ({ signal }: { readonly signal: AbortSignal }) =>
        run(
          Daemon.use((daemon) => daemon.skill(d.id)),
          { signal },
        ),
      enabled: isExpanded(d),
    })),
  });
  const servers = useQueries({
    queries: allServers.map((d) => ({
      queryKey: queryKeys.tools(toolsDirectory, d.id),
      queryFn: ({ signal }: { readonly signal: AbortSignal }) =>
        run(
          Daemon.use((daemon) => daemon.tools(toolsDirectory, d.id)),
          { signal },
        ),
      enabled: toolsDirectory !== "" && isExpanded(d),
      retry: false,
    })),
  });
  const skillFiles = new Map<string, Loading<GetSkillResponse>>(
    allSkills.flatMap((d, index) => {
      const query = skills[index];
      return query === undefined ? [] : [[d.id, query]];
    }),
  );
  const serverTools = new Map<string, Loading<McpTools>>(
    allServers.flatMap((d, index) => {
      const query = servers[index];
      return query === undefined ? [] : [[d.id, query]];
    }),
  );

  const targets = new Map<string, Target>();
  const lazy = new Map<string, boolean>();
  const decorations = new Map<string, string>();
  const icons = new Map<string, TreeIcon>([[filesFolder, filesIcon]]);
  const bundleIcon = sectionIcon("bundles");
  // Folders that stand for a kind, at the top or inside a bundle or file.
  const kindFolder = (folder: string, kind: string) => {
    const icon = sectionIcon(kind);
    if (icon !== undefined) {
      icons.set(folder, icon);
    }
  };
  if (grouping === "none" && byType) {
    for (const { kind } of kinds) {
      kindFolder(label(kind), kind);
      // An empty kind offers to add one, so the tree is also where things start.
      if (definitions.isSuccess && !filtering && !shown.some((d) => d.kind === kind)) {
        targets.set(`${label(kind)}/Add ${one(kind)}…`, { page: "new", kind });
      }
    }
  }
  for (const definition of shown) {
    const here = enabledHere?.has(`${definition.kind}/${definition.id}`) === true;
    const builtin = configuration.data?.harnesses.find((h) => h.name === definition.id)?.builtin;
    const icon =
      (definition.kind === "harnesses" && builtin !== undefined
        ? harnessIcon(builtin)
        : undefined) ??
      definitionIcon(
        definition.kind,
        definition.id,
        definition.description,
        definition.color,
        definition.icon,
      );
    for (const group of groupsOf(definition)) {
      const folder = join(group, byType ? label(definition.kind) : "");
      const path = join(folder, nameOf(definition));
      if (byType) {
        kindFolder(folder, definition.kind);
      }
      if (grouping === "bundle" && group !== outsideBundles && bundleIcon !== undefined) {
        icons.set(group, bundleIcon);
      }
      if (icon !== undefined) {
        icons.set(path, icon);
      }
      // Folders an ID's slashes make: a package's name, or a plain group.
      const segments = definition.id.split("/");
      for (let depth = 1; depth < segments.length; depth++) {
        const prefix = segments.slice(0, depth).join("/");
        icons.set(join(folder, prefix), namespaceIcon(prefix === definition.package));
      }
      if (definition.readOnly && definition.package === "") {
        decorations.set(path, "read-only");
      } else if (here && !filtering && directory !== "") {
        decorations.set(path, "here");
      }
      switch (definition.kind) {
        case "skills": {
          targets.set(path, { page: "skill", skill: definition.id });
          const loaded = skillFiles.get(definition.id);
          lazy.set(path, loaded?.data !== undefined || (loaded?.error ?? null) !== null);
          if ((loaded?.error ?? null) !== null) {
            decorations.set(path, "failed to load");
          }
          if (loaded?.data !== undefined) {
            const files = new Set(["SKILL.md", ...loaded.data.files]);
            for (const file of files) {
              targets.set(`${path}/${file}`, { page: "skill", skill: definition.id, file });
            }
          }
          break;
        }
        case "mcp": {
          targets.set(path, { page: "server", server: definition.id });
          const loaded = serverTools.get(definition.id);
          lazy.set(path, loaded?.data !== undefined || (loaded?.error ?? null) !== null);
          if ((loaded?.error ?? null) !== null) {
            const needsLogin =
              loaded?.error instanceof DaemonError && loaded.error.reason === "needs_login";
            decorations.set(
              path,
              mcpStates[needsLogin ? McpRuntime_State.NEEDS_AUTH : McpRuntime_State.FAILED].label,
            );
          }
          // A running server's icon is the latest; the definition carries the one remembered.
          if (loaded?.data !== undefined && loaded.data.icon !== "") {
            icons.set(path, { image: loaded.data.icon });
          }
          for (const tool of loaded?.data?.tools ?? []) {
            icons.set(`${path}/${tool.name}`, toolIcon(tool.name));
            targets.set(`${path}/${tool.name}`, {
              page: "server",
              server: definition.id,
              tool: tool.name,
            });
          }
          break;
        }
        default:
          targets.set(path, { page: "definition", kind: definition.kind, id: definition.id });
      }
    }
  }
  // Grouped by bundle, a bundle's group opens the bundle.
  if (grouping === "bundle") {
    for (const bundle of shown.filter((d) => d.kind === "bundles")) {
      targets.set(bundle.id, { page: "definition", kind: "bundles", id: bundle.id });
      if (bundleIcon !== undefined) {
        icons.set(bundle.id, bundleIcon);
      }
    }
  }
  // Grouped by file, each file is its own folder and opens itself; otherwise files have theirs.
  for (const file of configuration.data?.files ?? []) {
    const path =
      grouping === "file" ? relative(file.path) : `${filesFolder}/${relative(file.path)}`;
    targets.set(path, { page: "file", file: file.path });
    if (grouping === "file") {
      icons.set(path, filesIcon);
    }
    if (file.readOnly) {
      decorations.set(path, "read-only");
    }
  }

  // The page's item, at its first place in the tree. A skill's file or a server's tool is
  // selected before it loads, so its folder opens for it.
  const itemLabel = search.kind === undefined ? undefined : label(search.kind);
  const definition =
    search.kind === undefined || search.id === undefined
      ? undefined
      : shown.find((d) => d.kind === search.kind && d.id === search.id);
  const selected =
    search.file !== undefined
      ? [...targets].find(
          ([, target]) => target.page === "file" && target.file === search.file,
        )?.[0]
      : itemLabel === undefined || search.id === undefined
        ? undefined
        : `${definition === undefined ? `${itemLabel}/${search.id}` : (placesOf(definition)[0] ?? definition.id)}${
            search.kind === "skills" && search.path !== undefined
              ? `/${search.path}`
              : search.kind === "mcp" && search.tool !== undefined
                ? `/${search.tool}`
                : ""
          }`;

  const order =
    grouping === "none"
      ? byType
        ? kinds.map((k) => k.label)
        : []
      : grouping === "bundle"
        ? all.filter((d) => d.kind === "bundles").map((d) => d.id)
        : // The root first; it lists the others.
          [relative(configuration.data?.root ?? "")];

  return (
    <NavTree
      // A remount starts the new grouping with its groups open, and only those: a server's
      // folder opening starts the server.
      key={`${grouping} ${byType}`}
      expansion={Number(grouping !== "none") + Number(byType)}
      // Lazy folders hold what is loaded; everything else is a leaf.
      // A target that holds others, such as a file grouped by, is their folder rather than a row.
      paths={[...targets.keys()].filter(
        (path) =>
          !lazy.has(path) && ![...targets.keys()].some((other) => other.startsWith(`${path}/`)),
      )}
      lazy={lazy}
      onExpand={(folder) => setExpanded((current) => new Set([...current, folder]))}
      order={order}
      last={grouping === "file" ? [] : [outsideBundles, filesFolder]}
      // Files and definitions are rows to open, not sections.
      sections={grouping === "bundle" || (grouping === "none" && byType)}
      icons={icons}
      selected={selected}
      decorations={decorations}
      onSelect={(path) => {
        const target = targets.get(path);
        if (target === undefined) {
          return;
        }
        // Moving between definitions keeps the tab, so several can be edited in a row.
        const tab = search.tab === "definition" ? { tab: "definition" as const } : {};
        switch (target.page) {
          case "skill":
            void navigate({
              to: "/config",
              search:
                target.file === undefined || target.file === "SKILL.md"
                  ? { kind: "skills", id: target.skill, ...tab }
                  : { kind: "skills", id: target.skill, path: target.file },
            });
            return;
          case "server":
            void navigate({
              to: "/config",
              search:
                target.tool === undefined
                  ? { kind: "mcp", id: target.server, ...tab }
                  : { kind: "mcp", id: target.server, tool: target.tool },
            });
            return;
          case "definition":
            void navigate({ to: "/config", search: { kind: target.kind, id: target.id, ...tab } });
            return;
          case "file":
            void navigate({ to: "/config", search: { file: target.file } });
            return;
          case "new":
            void navigate({ to: "/config", search: { kind: target.kind, create: true } });
        }
      }}
      query={query}
      header={
        <div className="flex flex-col gap-2 p-2">
          <div className="flex items-center gap-1">
            <InputGroup>
              <InputGroupAddon>
                <SearchIcon />
              </InputGroupAddon>
              <InputGroupInput
                type="search"
                aria-label="Search the configuration"
                placeholder="Search configuration…"
                autoComplete="off"
                spellCheck={false}
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
            </InputGroup>
            <NewItemMenu />
          </div>
          <TreeOption label="Group by">
            <ToggleGroup
              variant="outline"
              size="sm"
              value={[grouping]}
              onValueChange={(next) => {
                const value = groupings.find((g) => next.includes(g.value))?.value;
                if (value !== undefined) {
                  globalThis.localStorage?.setItem(groupingKey, value);
                  setGrouping(value);
                }
              }}
            >
              {groupings.map((g) => (
                <ToggleGroupItem key={g.value} value={g.value}>
                  {g.label}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </TreeOption>
          <TreeOption label="Split by type">
            <Switch
              size="sm"
              aria-label="Split by type"
              checked={byType}
              onCheckedChange={(checked) => {
                globalThis.localStorage?.setItem(byTypeKey, String(checked));
                setByType(checked);
              }}
            />
          </TreeOption>
          {directory !== "" && (
            <TreeOption label="Show">
              <ToggleGroup
                variant="outline"
                size="sm"
                value={[onlyHere ? "here" : "all"]}
                onValueChange={(next) => {
                  if (next.length > 0) {
                    setOnlyHere(next.includes("here"));
                  }
                }}
              >
                <ToggleGroupItem value="all">All</ToggleGroupItem>
                <ToggleGroupItem value="here">In scope</ToggleGroupItem>
              </ToggleGroup>
            </TreeOption>
          )}
        </div>
      }
    />
  );
}

function TreeOption({ label, children }: { readonly label: string; readonly children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-muted-foreground text-xs">{label}</span>
      {children}
    </div>
  );
}
