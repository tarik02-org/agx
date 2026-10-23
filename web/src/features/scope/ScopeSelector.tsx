import { useNavigate } from "@tanstack/react-router";
import { FolderIcon, GlobeIcon } from "lucide-react";
import { useState } from "react";

import { Button } from "~/components/ui/button.tsx";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "~/components/ui/dialog.tsx";
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
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger
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
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-3xl">
        <DialogHeader className="border-b px-4 py-3">
          <DialogTitle>Scope</DialogTitle>
        </DialogHeader>
        <DirectoryBrowser
          key={String(open)}
          value={directory}
          home={home}
          shortcuts={configuration.data?.scopeDirectories ?? []}
          everywhere
          wide
          onPick={scopeTo}
        />
      </DialogContent>
    </Dialog>
  );
}
