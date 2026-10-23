import { useQueries } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { cn } from "cn";
import { FilterIcon, SearchIcon } from "lucide-react";
import { useState } from "react";

import { NavTree, type TreeIcon } from "~/components/NavTree.tsx";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "~/components/ui/input-group.tsx";
import { Tooltip, TooltipContent, TooltipTrigger } from "~/components/ui/tooltip.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { DaemonError } from "~/daemon/error.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { useRunEffect } from "~/effect/react.tsx";
import { kinds } from "~/features/config/kinds.ts";
import {
  definitionIcon,
  filesIcon,
  harnessIcon,
  namespaceIcon,
  sectionIcon,
  toolIcon,
} from "~/features/config/treeIcons.ts";
import { useInspectorDirectory } from "~/features/mcp/tools.ts";
import { mcpStates } from "~/features/runtime/states.ts";
import { useEnabledHere, useScopeDirectory } from "~/features/scope/scope.ts";
import type { GetSkillResponse, McpTools } from "~/gen/agx/v1/agx_pb.ts";
import { McpRuntime_State } from "~/gen/agx/v1/agx_pb.ts";
import { shortPath } from "~/lib/path.ts";

/** Kinds a scope selects; the others apply wherever they are defined. */
const scoped = new Set(["skills", "instructions", "mcp"]);

const label = (kind: string) => kinds.find((k) => k.kind === kind)?.label ?? kind;

/** What the tree needs of a folder's query. */
interface Loading<A> {
  readonly data: A | undefined;
  readonly error: unknown;
}

type Target =
  | { readonly page: "skill"; readonly skill: string; readonly file?: string }
  | { readonly page: "server"; readonly server: string; readonly tool?: string }
  | { readonly page: "definition"; readonly kind: string; readonly id: string }
  | { readonly page: "file"; readonly file: string };

/**
 * The configuration as one tree: a folder per kind with its definitions, IDs nesting at their
 * slashes (a package's skills sit under the package's name), and the configuration files. Skills
 * hold their files and MCP servers their tools, loaded when first expanded; a server starts only
 * then.
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
  const [onlyHere, setOnlyHere] = useState(true);
  const [query, setQuery] = useState("");
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set());
  const navigate = useNavigate();
  const search = useSearch({ strict: false });
  const run = useRunEffect();
  const home = configuration.data?.home ?? "";

  const shown = (definitions.data ?? []).filter(
    (definition) =>
      enabledHere === undefined ||
      !onlyHere ||
      !scoped.has(definition.kind) ||
      enabledHere.has(`${definition.kind}/${definition.id}`),
  );
  // Every skill and server has its query, run once its folder is expanded; one a page already
  // read, under the same key, shows at once, with the icon its server declares.
  const allSkills = shown.filter((d) => d.kind === "skills");
  const allServers = shown.filter((d) => d.kind === "mcp");
  const skills = useQueries({
    queries: allSkills.map((d) => ({
      queryKey: queryKeys.skill(d.id),
      queryFn: ({ signal }: { readonly signal: AbortSignal }) =>
        run(
          Daemon.use((daemon) => daemon.skill(d.id)),
          { signal },
        ),
      enabled: expanded.has(`${label("skills")}/${d.id}`),
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
      enabled: toolsDirectory !== "" && expanded.has(`${label("mcp")}/${d.id}`),
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
  const icons = new Map<string, TreeIcon>([["Files", filesIcon]]);
  for (const { kind } of kinds) {
    const icon = sectionIcon(kind);
    if (icon !== undefined) {
      icons.set(label(kind), icon);
    }
  }
  for (const definition of shown) {
    const here = enabledHere?.has(`${definition.kind}/${definition.id}`) === true;
    const path = `${label(definition.kind)}/${definition.id}`;
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
    if (icon !== undefined) {
      icons.set(path, icon);
    }
    // Folders an ID's slashes make: a package's name, or a plain group.
    const segments = definition.id.split("/");
    for (let depth = 1; depth < segments.length; depth++) {
      const prefix = segments.slice(0, depth).join("/");
      icons.set(
        `${label(definition.kind)}/${prefix}`,
        namespaceIcon(prefix === definition.package),
      );
    }
    if (definition.readOnly && definition.package === "") {
      decorations.set(path, "read-only");
    } else if (here && !onlyHere) {
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
  for (const file of configuration.data?.files ?? []) {
    const path = `Files/${shortPath(file.path, home).replace(/^~\//, "")}`;
    targets.set(path, { page: "file", file: file.path });
    if (file.readOnly) {
      decorations.set(path, "read-only");
    }
  }
  // A skill's file or a server's tool is selected before it loads, so its folder opens for it.
  const selected =
    search.skill !== undefined
      ? `${label("skills")}/${search.skill}${search.file === undefined ? "" : `/${search.file}`}`
      : search.server !== undefined
        ? `${label("mcp")}/${search.server}${search.tool === undefined ? "" : `/${search.tool}`}`
        : [...targets].find(([, target]) => {
            switch (target.page) {
              case "skill":
              case "server":
                return false;
              case "definition":
                return search.kind === target.kind && search.id === target.id;
              case "file":
                return search.file === target.file;
            }
          })?.[0];

  return (
    <NavTree
      // Lazy folders hold what is loaded; everything else is a leaf.
      paths={[...targets.keys()].filter((path) => !lazy.has(path))}
      lazy={lazy}
      onExpand={(folder) => setExpanded((current) => new Set([...current, folder]))}
      order={[...kinds.map((k) => k.label), "Files"]}
      icons={icons}
      selected={selected}
      decorations={decorations}
      onSelect={(path) => {
        const target = targets.get(path);
        if (target === undefined) {
          return;
        }
        switch (target.page) {
          case "skill":
            void navigate({
              to: "/skills",
              search: target.file === undefined ? { skill: target.skill } : { ...target },
            });
            return;
          case "server":
            void navigate({
              to: "/mcp",
              search: target.tool === undefined ? { server: target.server } : { ...target },
            });
            return;
          case "definition":
            void navigate({ to: "/config", search: { kind: target.kind, id: target.id } });
            return;
          case "file":
            void navigate({ to: "/config", search: { file: target.file } });
        }
      }}
      query={query}
      header={
        <div className="px-2 pt-1 pb-2">
          <InputGroup>
            <InputGroupAddon>
              <SearchIcon />
            </InputGroupAddon>
            <InputGroupInput
              type="search"
              aria-label="Search the configuration"
              placeholder="Search…"
              autoComplete="off"
              spellCheck={false}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
            {directory !== "" && (
              <InputGroupAddon align="inline-end">
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <InputGroupButton
                        size="icon-xs"
                        variant={onlyHere ? "secondary" : "ghost"}
                        aria-label="Only what the scope directory gets"
                        aria-pressed={onlyHere}
                        className={cn(onlyHere && "text-primary")}
                        onClick={() => setOnlyHere((current) => !current)}
                      >
                        <FilterIcon />
                      </InputGroupButton>
                    }
                  />
                  <TooltipContent side="right">
                    {onlyHere
                      ? `Showing only what ${shortPath(directory, home)} gets`
                      : `Show only what ${shortPath(directory, home)} gets`}
                  </TooltipContent>
                </Tooltip>
              </InputGroupAddon>
            )}
          </InputGroup>
        </div>
      }
    />
  );
}
