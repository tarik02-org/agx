import { Link } from "@tanstack/react-router";
import {
  ActivityIcon,
  BlocksIcon,
  FileCode2Icon,
  type LucideIcon,
  ScanSearchIcon,
  WrenchIcon,
} from "lucide-react";
import type { ReactNode } from "react";

import {
  Sidebar,
  SidebarContent,
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

interface Section {
  readonly label: string;
  readonly icon: LucideIcon;
  readonly to: "/" | "/explain" | null;
}

// Pages arrive in the order ADR 0026 lists them; those still to come show as such.
const sections: ReadonlyArray<Section> = [
  { label: "Runtime", icon: ActivityIcon, to: "/" },
  { label: "Explain", icon: ScanSearchIcon, to: "/explain" },
  { label: "MCP inspector", icon: WrenchIcon, to: null },
  { label: "Skills", icon: BlocksIcon, to: null },
  { label: "Configuration", icon: FileCode2Icon, to: null },
];

export function AppShell({ children }: { readonly children: ReactNode }) {
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
                        {section.to === null ? (
                          <SidebarMenuButton disabled tooltip={section.label}>
                            <section.icon />
                            <span>{section.label}</span>
                          </SidebarMenuButton>
                        ) : (
                          <SidebarMenuButton
                            tooltip={section.label}
                            render={<Link to={section.to} activeProps={{ "data-active": true }} />}
                          >
                            <section.icon />
                            <span>{section.label}</span>
                          </SidebarMenuButton>
                        )}
                        {section.to === null && <SidebarMenuBadge>soon</SidebarMenuBadge>}
                      </SidebarMenuItem>
                    ))}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            </SidebarContent>
          </Sidebar>
          <SidebarInset>{children}</SidebarInset>
        </SidebarProvider>
      </TooltipProvider>
    </Toaster>
  );
}
