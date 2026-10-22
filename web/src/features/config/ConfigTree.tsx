import { Link } from "@tanstack/react-router";
import { ChevronRightIcon, LockIcon, PlusIcon } from "lucide-react";
import { useState } from "react";

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "~/components/ui/collapsible.tsx";
import { Field, FieldLabel } from "~/components/ui/field.tsx";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInput,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from "~/components/ui/sidebar.tsx";
import { Switch } from "~/components/ui/switch.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { kinds } from "~/features/config/kinds.ts";
import { useEnabledHere, useScopeDirectory } from "~/features/scope/scope.ts";
import type { Definition } from "~/gen/agx/v1/agx_pb.ts";
import { shortPath } from "~/lib/path.ts";

/** Kinds a scope selects; the others apply wherever they are defined. */
const scoped = new Set(["skills", "instructions", "mcp"]);

const active = { "data-active": true, "aria-current": "page" } as const;

// Skills open on the skills page, which shows their files; everything else in the editor. The
// sidebar button renders as this link and merges its own props into it.
const definitionLink = (definition: Definition) =>
  definition.kind === "skills" ? (
    <Link to="/skills" search={{ skill: definition.id }} activeProps={active} />
  ) : (
    <Link to="/config" search={{ kind: definition.kind, id: definition.id }} activeProps={active} />
  );

/** The whole configuration as a tree: every definition by kind, package skills under their package. */
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
  const enabledHere = useEnabledHere();
  const [onlyHere, setOnlyHere] = useState(true);
  const [filter, setFilter] = useState("");
  const home = configuration.data?.home ?? "";
  const all = definitions.data ?? [];
  const shown = (definition: Definition): boolean => {
    if (filter !== "" && !definition.id.toLowerCase().includes(filter.toLowerCase())) {
      return false;
    }
    if (enabledHere === undefined || !onlyHere || !scoped.has(definition.kind)) {
      return true;
    }
    return enabledHere.has(`${definition.kind}/${definition.id}`);
  };
  const packageSkills = (packageId: string) =>
    all.filter((d) => d.kind === "skills" && d.package === packageId && shown(d));
  return (
    <Sidebar
      collapsible="offcanvas"
      className="top-(--header-height) h-[calc(100svh-var(--header-height))]!"
    >
      <SidebarHeader>
        <SidebarInput
          type="search"
          aria-label="Filter configuration"
          placeholder="Filter configuration…"
          autoComplete="off"
          spellCheck={false}
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
        />
        {directory !== "" && (
          <Field orientation="horizontal" className="px-1">
            <Switch id="only-here" checked={onlyHere} onCheckedChange={setOnlyHere} />
            <FieldLabel htmlFor="only-here">Only what this directory gets</FieldLabel>
          </Field>
        )}
      </SidebarHeader>
      <SidebarContent>
        {definitions.isPending && (
          <SidebarMenu className="px-2">
            <SidebarMenuSkeleton />
            <SidebarMenuSkeleton />
            <SidebarMenuSkeleton />
          </SidebarMenu>
        )}
        {kinds.map(({ kind, label }) => {
          // Package skills show under their package.
          const own = all.filter(
            (d) => d.kind === kind && shown(d) && !(kind === "skills" && d.package !== ""),
          );
          const packages = kind === "packages" ? own : [];
          return (
            <Collapsible key={kind} defaultOpen className="group/collapsible">
              <SidebarGroup>
                <SidebarGroupLabel render={<CollapsibleTrigger />}>
                  <ChevronRightIcon className="transition-transform group-data-[open]/collapsible:rotate-90" />
                  {label}
                </SidebarGroupLabel>
                <SidebarGroupAction
                  render={<Link to="/config" search={{ kind, create: true }} />}
                  aria-label={`New ${label}`}
                >
                  <PlusIcon />
                </SidebarGroupAction>
                <CollapsibleContent>
                  <SidebarGroupContent>
                    <SidebarMenu>
                      {own.map((definition) => (
                        <SidebarMenuItem key={definition.id}>
                          <SidebarMenuButton
                            size="sm"
                            className="font-mono"
                            render={definitionLink(definition)}
                          >
                            <span className="truncate">{definition.id}</span>
                          </SidebarMenuButton>
                          {definition.readOnly && definition.package === "" && (
                            <SidebarMenuBadge>
                              <LockIcon aria-label="read-only" className="size-3" />
                            </SidebarMenuBadge>
                          )}
                          {kind === "packages" && packages.includes(definition) && (
                            <SidebarMenuSub>
                              {packageSkills(definition.id).map((skill) => (
                                <SidebarMenuSubItem key={skill.id}>
                                  <SidebarMenuSubButton
                                    size="sm"
                                    className="font-mono"
                                    render={definitionLink(skill)}
                                  >
                                    <span className="truncate">
                                      {skill.id.slice(definition.id.length + 1)}
                                    </span>
                                  </SidebarMenuSubButton>
                                </SidebarMenuSubItem>
                              ))}
                            </SidebarMenuSub>
                          )}
                        </SidebarMenuItem>
                      ))}
                      {own.length === 0 && (
                        <SidebarMenuItem>
                          <span className="px-2 text-muted-foreground text-xs">None</span>
                        </SidebarMenuItem>
                      )}
                    </SidebarMenu>
                  </SidebarGroupContent>
                </CollapsibleContent>
              </SidebarGroup>
            </Collapsible>
          );
        })}
        <SidebarGroup>
          <SidebarGroupLabel>Files</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {(configuration.data?.files ?? []).map((file) => (
                <SidebarMenuItem key={file.path}>
                  <SidebarMenuButton
                    size="sm"
                    className="font-mono"
                    render={<Link to="/config" search={{ file: file.path }} activeProps={active} />}
                  >
                    <span className="truncate">{shortPath(file.path, home)}</span>
                  </SidebarMenuButton>
                  {file.readOnly && (
                    <SidebarMenuBadge>
                      <LockIcon aria-label="read-only" className="size-3" />
                    </SidebarMenuBadge>
                  )}
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  );
}
