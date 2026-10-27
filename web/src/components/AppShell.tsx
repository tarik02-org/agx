import { Link, useBlocker, useLocation } from "@tanstack/react-router";
import { ActivityIcon, type LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarSeparator,
} from "~/components/ui/sidebar.tsx";
import { Toaster } from "~/components/ui/toast.tsx";
import { TooltipProvider } from "~/components/ui/tooltip.tsx";
import { ConfigTree } from "~/features/config/ConfigTree.tsx";
import { ReviewChanges } from "~/features/config/ReviewChanges.tsx";
import { useStaging } from "~/features/config/staging.ts";
import { ScopeControl } from "~/features/scope/ScopeControl.tsx";

interface Section {
  readonly label: string;
  readonly to: "/";
  readonly icon: LucideIcon;
}

// Pages that are not one item of the configuration; every item opens from the tree.
const sections: ReadonlyArray<Section> = [{ label: "Runtime", to: "/", icon: ActivityIcon }];

/**
 * The sidebar holds the scope, the pages, and the configuration as a tree, which the scope marks.
 * Each page has the one header, with its place, its actions, and the staged changes.
 */
export function AppShell({ children }: { readonly children: ReactNode }) {
  const pathname = useLocation({ select: (location) => location.pathname });
  // Staged changes live only in this tab, so closing or reloading it asks first; they survive
  // moving between pages, so navigation is never blocked.
  useBlocker({
    shouldBlockFn: () => false,
    enableBeforeUnload: () => useStaging.getState().changes.length > 0,
  });
  return (
    <Toaster>
      <TooltipProvider>
        <a
          href="#main"
          className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2 focus:text-xs"
        >
          Skip to content
        </a>
        <SidebarProvider>
          <Sidebar collapsible="offcanvas">
            {/* As tall as the page header, so their borders meet. */}
            <SidebarHeader className="h-12 shrink-0 flex-row items-center border-b px-4 py-0">
              <Link to="/" className="font-mono font-semibold">
                agx
              </Link>
            </SidebarHeader>
            <div className="px-2 pt-2">
              <ScopeControl />
            </div>
            <SidebarMenu className="p-2">
              {sections.map((section) => (
                <SidebarMenuItem key={section.to}>
                  <SidebarMenuButton
                    isActive={pathname === section.to}
                    render={<Link to={section.to} />}
                  >
                    <section.icon />
                    <span>{section.label}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
            <SidebarSeparator className="mx-0" />
            <SidebarContent className="min-h-0">
              <ConfigTree />
            </SidebarContent>
          </Sidebar>
          <SidebarInset className="h-svh overflow-hidden">
            <div
              id="main"
              tabIndex={-1}
              className="flex min-h-0 flex-1 flex-col overflow-auto outline-none"
            >
              {children}
            </div>
          </SidebarInset>
          <ReviewChanges />
        </SidebarProvider>
      </TooltipProvider>
    </Toaster>
  );
}
