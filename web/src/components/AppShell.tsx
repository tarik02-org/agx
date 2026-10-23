import { Link, useLocation } from "@tanstack/react-router";
import {
  ActivityIcon,
  BlocksIcon,
  GitCompareArrowsIcon,
  type LucideIcon,
  PlugIcon,
  ScanSearchIcon,
  Settings2Icon,
} from "lucide-react";
import { type ReactNode, useEffect } from "react";

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuBadge,
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
import { ScopeSelector } from "~/features/scope/ScopeSelector.tsx";

interface Section {
  readonly label: string;
  readonly to: "/" | "/explain" | "/mcp" | "/skills" | "/config";
  readonly icon: LucideIcon;
}

const sections: ReadonlyArray<Section> = [
  { label: "Runtime", to: "/", icon: ActivityIcon },
  { label: "Explain", to: "/explain", icon: ScanSearchIcon },
  { label: "MCP inspector", to: "/mcp", icon: PlugIcon },
  { label: "Skills", to: "/skills", icon: BlocksIcon },
  { label: "Configuration", to: "/config", icon: Settings2Icon },
];

/**
 * Everything global lives in the sidebar, at fixed places: the name and scope at its top, the
 * pages, the configuration as a tree, and the staged changes at its foot. Each page has the one
 * header, with its place and its actions.
 */
export function AppShell({ children }: { readonly children: ReactNode }) {
  const staged = useStaging((state) => state.changes.length);
  const review = useStaging((state) => state.review);
  const pathname = useLocation({ select: (location) => location.pathname });
  // Staged changes live only in this tab, so closing or reloading it asks first.
  useEffect(() => {
    if (staged === 0) {
      return;
    }
    const warn = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [staged]);
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
              <span className="px-1 font-mono font-semibold">agx</span>
              <ScopeSelector />
            </SidebarHeader>
            <SidebarMenu className="px-2 py-1">
              {sections.map((section) => (
                <SidebarMenuItem key={section.to}>
                  <SidebarMenuButton
                    isActive={
                      section.to === "/" ? pathname === "/" : pathname.startsWith(section.to)
                    }
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
            <SidebarFooter className="border-t px-2 py-2">
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton onClick={() => review(true)}>
                    <GitCompareArrowsIcon />
                    <span>Staged changes</span>
                  </SidebarMenuButton>
                  {staged > 0 && <SidebarMenuBadge>{staged}</SidebarMenuBadge>}
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarFooter>
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
