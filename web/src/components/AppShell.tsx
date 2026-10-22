import { Link } from "@tanstack/react-router";
import { GitCompareArrowsIcon } from "lucide-react";
import { type ReactNode, useEffect } from "react";

import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import {
  NavigationMenu,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
} from "~/components/ui/navigation-menu.tsx";
import { Separator } from "~/components/ui/separator.tsx";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "~/components/ui/sidebar.tsx";
import { Toaster } from "~/components/ui/toast.tsx";
import { TooltipProvider } from "~/components/ui/tooltip.tsx";
import { ConfigTree } from "~/features/config/ConfigTree.tsx";
import { ReviewChanges } from "~/features/config/ReviewChanges.tsx";
import { useStaging } from "~/features/config/staging.ts";
import { ScopeSelector } from "~/features/scope/ScopeSelector.tsx";

interface Section {
  readonly label: string;
  readonly to: "/" | "/explain" | "/mcp" | "/skills" | "/config";
}

const sections: ReadonlyArray<Section> = [
  { label: "Runtime", to: "/" },
  { label: "Explain", to: "/explain" },
  { label: "MCP inspector", to: "/mcp" },
  { label: "Skills", to: "/skills" },
  { label: "Configuration", to: "/config" },
];

/**
 * The top bar navigates between pages and holds the scope and staged changes; the sidebar is the
 * configuration itself, as a tree.
 */
export function AppShell({ children }: { readonly children: ReactNode }) {
  const staged = useStaging((state) => state.changes.length);
  const review = useStaging((state) => state.review);
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
        <div className="[--header-height:--spacing(11)]">
          <SidebarProvider className="flex flex-col">
            <header className="sticky top-0 z-20 flex h-(--header-height) shrink-0 items-center gap-2 border-b bg-background px-2">
              <SidebarTrigger />
              <Separator orientation="vertical" className="h-4" />
              <span className="px-1 font-mono font-semibold">agx</span>
              <NavigationMenu>
                <NavigationMenuList>
                  {sections.map((section) => (
                    <NavigationMenuItem key={section.to}>
                      <NavigationMenuLink
                        render={
                          <Link
                            to={section.to}
                            activeOptions={{ exact: section.to === "/" }}
                            activeProps={{ "data-active": true, "aria-current": "page" }}
                          />
                        }
                      >
                        {section.label}
                      </NavigationMenuLink>
                    </NavigationMenuItem>
                  ))}
                </NavigationMenuList>
              </NavigationMenu>
              <div className="ml-auto flex items-center gap-2">
                <ScopeSelector />
                <Button variant="outline" size="sm" onClick={() => review(true)}>
                  <GitCompareArrowsIcon data-icon="inline-start" />
                  Staged changes
                  {staged > 0 && <Badge>{staged}</Badge>}
                </Button>
              </div>
            </header>
            <div className="flex flex-1">
              <ConfigTree />
              <SidebarInset
                id="main"
                tabIndex={-1}
                className="h-[calc(100svh-var(--header-height))] overflow-auto outline-none"
              >
                {children}
              </SidebarInset>
            </div>
            <ReviewChanges />
          </SidebarProvider>
        </div>
      </TooltipProvider>
    </Toaster>
  );
}
