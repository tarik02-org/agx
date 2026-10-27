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

interface Section {
  readonly label: string;
  readonly to: "/";
  readonly icon: LucideIcon;
}

// Pages that are not one item of the configuration; every item opens from the tree.
const sections: ReadonlyArray<Section> = [{ label: "Runtime", to: "/", icon: ActivityIcon }];

/**
 * The sidebar holds the pages and the configuration as a tree. Each page has the one header, with
 * the scope, its place, its actions, and the staged changes.
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
            <SidebarHeader className="gap-2 px-3 pt-3">
              <Link to="/" className="self-start px-1 font-mono font-semibold">
                agx
              </Link>
            </SidebarHeader>
            <SidebarMenu className="px-2 py-1">
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
            <SidebarContent className="min-h-0 py-1">
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
