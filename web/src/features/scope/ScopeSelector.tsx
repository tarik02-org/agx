import { useNavigate } from "@tanstack/react-router";
import { FolderIcon, GlobeIcon } from "lucide-react";
import { useState } from "react";

import { Button } from "~/components/ui/button.tsx";
import { Popover, PopoverContent, PopoverTrigger } from "~/components/ui/popover.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { DirectoryBrowser } from "~/features/scope/DirectoryBrowser.tsx";
import { useScopeDirectory } from "~/features/scope/scope.ts";
import { shortPath } from "~/lib/path.ts";

/** Picks the directory every page is scoped to, or everywhere. */
export function ScopeSelector() {
  const directory = useScopeDirectory();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const configuration = useDaemonQuery(
    queryKeys.configuration,
    Daemon.use((daemon) => daemon.configuration),
  );
  const home = configuration.data?.home ?? "";
  const scopeTo = (next: string) => {
    setOpen(false);
    void navigate({
      to: ".",
      search: (current) => ({ ...current, dir: next === "" ? undefined : next }),
    });
  };
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button variant="outline" size="sm" className="max-w-80 font-mono">
            {directory === "" ? (
              <GlobeIcon data-icon="inline-start" />
            ) : (
              <FolderIcon data-icon="inline-start" />
            )}
            <span className="truncate">
              {directory === "" ? "Everywhere" : shortPath(directory, home)}
            </span>
          </Button>
        }
      />
      <PopoverContent align="end" className="w-80 gap-0 p-0">
        <p className="px-3 pt-2.5 pb-1.5 text-muted-foreground text-xs">
          Pages show what a launch in this directory gets, and enable things here.
        </p>
        <DirectoryBrowser
          key={String(open)}
          value={directory}
          home={home}
          shortcuts={configuration.data?.scopeDirectories ?? []}
          everywhere
          onPick={scopeTo}
        />
      </PopoverContent>
    </Popover>
  );
}
