import { useNavigate } from "@tanstack/react-router";
import { FolderIcon, GlobeIcon } from "lucide-react";
import { useState } from "react";

import { Button } from "~/components/ui/button.tsx";
import { Popover, PopoverContent, PopoverTrigger } from "~/components/ui/popover.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { DirectoryPicker } from "~/features/explain/DirectoryPicker.tsx";
import { absoluteDirectory, useScopeDirectory } from "~/features/scope/scope.ts";
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
      <PopoverContent align="end" className="flex w-auto flex-col gap-2">
        <span className="text-muted-foreground text-xs">
          Pages show what a launch in this directory gets, and enable things here.
        </span>
        <DirectoryPicker
          value={directory === "" ? "" : shortPath(directory, home)}
          suggestions={configuration.data?.scopeDirectories ?? []}
          onChange={(next) => scopeTo(absoluteDirectory(next, home))}
          className="w-[28rem]"
        />
        <Button variant="ghost" size="sm" disabled={directory === ""} onClick={() => scopeTo("")}>
          <GlobeIcon data-icon="inline-start" />
          Everywhere
        </Button>
      </PopoverContent>
    </Popover>
  );
}
