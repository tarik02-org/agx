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

interface Section {
  readonly label: string;
  readonly icon: LucideIcon;
  readonly to: "/" | null;
}

// Pages arrive in the order ADR 0026 lists them; those still to come show as such.
const sections: ReadonlyArray<Section> = [
  { label: "Runtime", icon: ActivityIcon, to: "/" },
  { label: "Explain", icon: ScanSearchIcon, to: null },
  { label: "MCP inspector", icon: WrenchIcon, to: null },
  { label: "Skills", icon: BlocksIcon, to: null },
  { label: "Configuration", icon: FileCode2Icon, to: null },
];

export function AppShell({ children }: { readonly children: ReactNode }) {
  return (
    <div className="flex h-dvh">
      <nav className="flex w-48 shrink-0 flex-col gap-0.5 border-r bg-card/50 p-2">
        <div className="px-2 pt-1 pb-3 font-mono font-semibold text-sm tracking-tight">agx</div>
        {sections.map((section) =>
          section.to === null ? (
            <span
              key={section.label}
              className="flex items-center gap-2 rounded-md px-2 py-1 text-muted-foreground/50"
              title="Coming soon"
            >
              <section.icon className="size-3.5" />
              {section.label}
            </span>
          ) : (
            <Link
              key={section.label}
              to={section.to}
              className="flex items-center gap-2 rounded-md px-2 py-1 text-muted-foreground hover:bg-accent hover:text-foreground data-[status=active]:bg-accent data-[status=active]:text-foreground"
            >
              <section.icon className="size-3.5" />
              {section.label}
            </Link>
          ),
        )}
      </nav>
      <main className="min-w-0 flex-1 overflow-auto">{children}</main>
    </div>
  );
}
