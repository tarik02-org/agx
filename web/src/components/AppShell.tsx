import { Link } from "@tanstack/react-router";
import {
  ActivityIcon,
  BlocksIcon,
  FileCode2Icon,
  GitCompareArrowsIcon,
  type LucideIcon,
  ScanSearchIcon,
  WrenchIcon,
} from "lucide-react";
import type { ReactNode } from "react";

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from "~/components/ui/sidebar.tsx";
import { Toaster } from "~/components/ui/toast.tsx";
import { TooltipProvider } from "~/components/ui/tooltip.tsx";
import { ReviewChanges } from "~/features/config/ReviewChanges.tsx";
import { useStaging } from "~/features/config/staging.ts";

interface Section {
  readonly label: string;
  readonly icon: LucideIcon;
  readonly to: "/" | "/explain" | "/mcp" | "/skills" | "/config";
}

const sections: ReadonlyArray<Section> = [
  { label: "Runtime", icon: ActivityIcon, to: "/" },
  { label: "Explain", icon: ScanSearchIcon, to: "/explain" },
  { label: "MCP inspector", icon: WrenchIcon, to: "/mcp" },
  { label: "Skills", icon: BlocksIcon, to: "/skills" },
  { label: "Configuration", icon: FileCode2Icon, to: "/config" },
];

export function AppShell({ children }: { readonly children: ReactNode }) {
  const staged = useStaging((state) => state.changes.length);
  const review = useStaging((state) => state.review);
  return (
    <Toaster>
      <TooltipProvider>
        <SidebarProvider>
          <Sidebar collapsible="icon">
            <SidebarHeader>
              <span className="px-2 font-mono font-semibold">agx</span>
            </SidebarHeader>
            <SidebarContent>
              <SidebarGroup>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {sections.map((section) => (
                      <SidebarMenuItem key={section.label}>
                        <SidebarMenuButton
                          tooltip={section.label}
                          render={<Link to={section.to} activeProps={{ "data-active": true }} />}
                        >
                          <section.icon />
                          <span>{section.label}</span>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
                    ))}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            </SidebarContent>
            <SidebarFooter>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton tooltip="Staged changes" onClick={() => review(true)}>
                    <GitCompareArrowsIcon />
                    <span>Staged changes</span>
                  </SidebarMenuButton>
                  {staged > 0 && <SidebarMenuBadge>{staged}</SidebarMenuBadge>}
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarFooter>
          </Sidebar>
          <SidebarInset>{children}</SidebarInset>
          <ReviewChanges />
        </SidebarProvider>
      </TooltipProvider>
    </Toaster>
  );
}
