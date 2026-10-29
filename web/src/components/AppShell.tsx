import { Link, useBlocker, useLocation } from "@tanstack/react-router";
import { ActivityIcon } from "lucide-react";
import type { ReactNode } from "react";

import { ButtonLink } from "~/components/ButtonLink.tsx";
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarInset,
  SidebarProvider,
  SidebarSeparator,
} from "~/components/ui/sidebar.tsx";
import { Toaster } from "~/components/ui/toast.tsx";
import { TooltipProvider } from "~/components/ui/tooltip.tsx";
import { ConfigTree } from "~/features/config/ConfigTree.tsx";
import { NewItemMenu } from "~/features/config/NewItemMenu.tsx";
import { ReviewChanges } from "~/features/config/ReviewChanges.tsx";
import { useStaging } from "~/features/config/staging.ts";
import { ScopeControl } from "~/features/scope/ScopeControl.tsx";

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
            {/* The one page that is not an item of the configuration, and starting a new item,
                beside the name; every item opens from the tree. */}
            <SidebarHeader className="h-12 shrink-0 flex-row items-center gap-1 border-b py-0 pr-2 pl-4">
              <Link to="/" className="mr-auto font-mono font-semibold">
                agx
              </Link>
              <ButtonLink
                to="/"
                variant={pathname === "/" ? "secondary" : "ghost"}
                size="sm"
                aria-current={pathname === "/" ? "page" : undefined}
              >
                <ActivityIcon data-icon="inline-start" />
                Runtime
              </ButtonLink>
              <NewItemMenu />
            </SidebarHeader>
            <div className="p-2">
              <ScopeControl />
            </div>
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
